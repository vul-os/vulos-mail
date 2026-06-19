package smtpin_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/services/mtaout"
)

// Authenticated submission: AUTH PLAIN -> MAIL/RCPT/DATA stores a Sent copy and
// enqueues one OutMessage per destination domain.
func TestSubmissionStoresSentAndEnqueues(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var enqueued []mtaout.OutMessage
	be := &smtpin.SubmitBackend{
		Auth: func(u, p string) (*account.Runtime, string, error) {
			if u == "alice" && p == "secret" {
				return rt, "tenant-a", nil
			}
			return nil, "", gosmtp.ErrAuthFailed
		},
		Enqueue: func(m mtaout.OutMessage) { enqueued = append(enqueued, m) },
	}
	srv := smtpin.NewSubmitServer(be, "127.0.0.1:0", "vmail.test")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Auth(sasl.NewPlainClient("", "alice", "secret")); err != nil {
		t.Fatalf("auth: %v", err)
	}

	raw := []byte("From: alice@vmail.test\r\nTo: bob@gmail.com, carol@yahoo.com\r\nSubject: Hi\r\n\r\nbody\r\n")
	if err := c.Mail("alice@vmail.test", nil); err != nil {
		t.Fatal(err)
	}
	for _, rcpt := range []string{"bob@gmail.com", "carol@yahoo.com"} {
		if err := c.Rcpt(rcpt, nil); err != nil {
			t.Fatal(err)
		}
	}
	w, err := c.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Sent copy stored.
	sent := rt.MessagesWithLabel(model.LabelSent)
	if len(sent) != 1 {
		t.Fatalf("Sent should have 1 message, got %d", len(sent))
	}
	if !sent[0].Flags[model.FlagSeen] {
		t.Error("Sent copy should be marked Seen")
	}

	// One OutMessage per destination domain.
	if len(enqueued) != 2 {
		t.Fatalf("expected 2 enqueued (gmail.com, yahoo.com), got %d", len(enqueued))
	}
	domains := map[string]bool{}
	for _, m := range enqueued {
		domains[m.RcptDomain] = true
		if m.Tenant != "tenant-a" || m.FromDomain != "vmail.test" {
			t.Errorf("bad OutMessage fields: %+v", m)
		}
	}
	if !domains["gmail.com"] || !domains["yahoo.com"] {
		t.Errorf("destination domains = %v", domains)
	}
}

// Submission without auth is rejected.
func TestSubmissionRequiresAuth(t *testing.T) {
	be := &smtpin.SubmitBackend{
		Auth:    func(string, string) (*account.Runtime, string, error) { return nil, "", gosmtp.ErrAuthFailed },
		Enqueue: func(mtaout.OutMessage) {},
	}
	sess, _ := be.NewSession(nil)
	if err := sess.Mail("x@y.com", nil); err == nil {
		t.Error("MAIL before auth should fail")
	}
}
