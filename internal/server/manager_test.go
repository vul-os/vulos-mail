package server_test

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/services/mtaout"
)

type okSender struct{ n int }

func (s *okSender) Send(context.Context, mtaout.OutMessage, string) mtaout.SendResult {
	s.n++
	return mtaout.SendResult{Status: mtaout.Delivered}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// Exercises the whole system through real protocol clients: a message arrives via
// the MX, is read back over IMAP, and an authenticated submission is scheduled
// and delivered through mtaout.
func TestEndToEndAllProtocols(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	blobs, err := blob.NewFS(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	sender := &okSender{}
	sched := mtaout.NewScheduler(mtaout.Config{Sender: sender, MaxPerDomain: 10})
	mgr := server.NewManager(dir, blobs, sched)
	mgr.AddAccount("alice@vmail.test", "pw")

	// --- 1. Inbound via MX ---
	mxLn := listen(t)
	defer mxLn.Close()
	go smtpin.NewServer(&smtpin.Backend{Deliver: mgr.Deliver}, "", "vmail.test").Serve(mxLn)

	mc, err := gosmtp.Dial(mxLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	raw := "From: ext@out.example\r\nTo: alice@vmail.test\r\nSubject: Inbound hello\r\nMessage-ID: <in1@out>\r\n\r\nhi alice\r\n"
	if err := mc.SendMail("ext@out.example", []string{"alice@vmail.test"}, strings.NewReader(raw)); err != nil {
		t.Fatalf("mx send: %v", err)
	}
	mc.Close()

	// --- 2. Read it back over IMAP ---
	imapLn := listen(t)
	defer imapLn.Close()
	imapBe := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}
	go imapadapter.NewServer(imapBe, nil).Serve(imapLn)

	ic, err := imapclient.DialInsecure(imapLn.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ic.Close()
	if err := ic.Login("alice@vmail.test", "pw").Wait(); err != nil {
		t.Fatalf("imap login: %v", err)
	}
	sel, err := ic.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatalf("imap select: %v", err)
	}
	if sel.NumMessages != 1 {
		t.Fatalf("inbox = %d messages, want 1 (MX->IMAP path broken)", sel.NumMessages)
	}
	bufs, err := ic.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{Envelope: true}).Collect()
	if err != nil || len(bufs) != 1 || bufs[0].Envelope.Subject != "Inbound hello" {
		t.Fatalf("fetch wrong: %v / %+v", err, bufs)
	}
	_ = ic.Logout().Wait()

	// --- 3. Authenticated submission -> scheduler -> delivered ---
	subLn := listen(t)
	defer subLn.Close()
	subBe := &smtpin.SubmitBackend{Auth: mgr.AuthSubmit, Enqueue: mgr.Enqueue}
	go smtpin.NewSubmitServer(subBe, "", "vmail.test").Serve(subLn)

	sc, err := gosmtp.Dial(subLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	if err := sc.Auth(sasl.NewPlainClient("", "alice@vmail.test", "pw")); err != nil {
		t.Fatalf("submit auth: %v", err)
	}
	out := "From: alice@vmail.test\r\nTo: bob@gmail.com\r\nSubject: Outbound\r\n\r\nhello bob\r\n"
	if err := sc.SendMail("alice@vmail.test", []string{"bob@gmail.com"}, strings.NewReader(out)); err != nil {
		t.Fatalf("submit send: %v", err)
	}

	if sched.Pending() != 1 {
		t.Fatalf("scheduler pending = %d, want 1", sched.Pending())
	}
	if st := sched.Tick(ctx, time.Unix(0, 0).UTC()); st.Delivered != 1 {
		t.Fatalf("tick delivered = %d, want 1", st.Delivered)
	}
	if sender.n != 1 {
		t.Errorf("sender called %d times, want 1", sender.n)
	}
}
