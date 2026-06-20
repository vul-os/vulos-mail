package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	jmapadapter "github.com/vul-os/vmail/adapters/jmap"
	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/services/mtaout"
)

// loopSender closes the outbound loop: instead of dialing the internet, it
// delivers mail straight back into the local MX, so a vmail->vmail send completes
// fully offline. This is what lets one test simulate the entire mail lifecycle.
type loopSender struct{ mgr *server.Manager }

func (s *loopSender) Send(ctx context.Context, msg mtaout.OutMessage, _ string) mtaout.SendResult {
	for _, rcpt := range msg.Rcpts {
		if err := s.mgr.Deliver(ctx, rcpt, msg.Raw); err != nil {
			return mtaout.SendResult{Status: mtaout.PermFail, Err: err}
		}
	}
	return mtaout.SendResult{Status: mtaout.Delivered}
}

// TestFullStackSimulation drives the complete system through real protocol
// clients: external mail in via MX, an internal alice->bob send that loops
// through submission + scheduler + MX, and bob reading the result over BOTH IMAP
// and JMAP. Every adapter, the runtime, deliverability, and DKIM signing are
// exercised together.
func TestFullStackSimulation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	blobs, err := blob.NewFS(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}

	ls := &loopSender{}
	sched := mtaout.NewScheduler(mtaout.Config{Sender: ls, MaxPerDomain: 20})
	mgr := server.NewManager(dir, blobs, sched)
	ls.mgr = mgr
	if _, err := mgr.EnsureDKIM("vmail.test", "s1"); err != nil {
		t.Fatal(err)
	}
	_ = mgr.AddAccount("alice@vmail.test", "alicepw")
	_ = mgr.AddAccount("bob@vmail.test", "bobpw")

	// --- listeners ---
	mxLn, subLn, imapLn := mustListen(t), mustListen(t), mustListen(t)
	defer mxLn.Close()
	defer subLn.Close()
	defer imapLn.Close()
	go smtpin.NewServer(&smtpin.Backend{Deliver: mgr.Deliver, AuthServID: "vmail.test"}, "", "vmail.test").Serve(mxLn)
	go smtpin.NewSubmitServer(&smtpin.SubmitBackend{Auth: mgr.AuthSubmit, Enqueue: mgr.Enqueue, Signer: mgr.Signer}, "", "vmail.test").Serve(subLn)
	imapBe := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}
	go imapadapter.NewServer(imapBe, nil).Serve(imapLn)
	jmapSrv := httptest.NewServer((&jmapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}).Handler())
	defer jmapSrv.Close()

	// --- 1. external -> alice (MX) ---
	smtpSend(t, mxLn.Addr().String(), "", "", "boss@out.example", "alice@vmail.test",
		"From: boss@out.example\r\nTo: alice@vmail.test\r\nSubject: Welcome\r\n\r\nwelcome alice\r\n")
	if n := imapInboxCount(t, imapLn.Addr().String(), "alice@vmail.test", "alicepw"); n != 1 {
		t.Fatalf("alice inbox after external mail = %d, want 1", n)
	}

	// --- 2. alice -> bob (submission -> DKIM -> scheduler -> loop -> MX) ---
	smtpSend(t, subLn.Addr().String(), "alice@vmail.test", "alicepw", "alice@vmail.test", "bob@vmail.test",
		"From: alice@vmail.test\r\nTo: bob@vmail.test\r\nSubject: Lunch?\r\n\r\nfree at noon?\r\n")
	if got := sched.Tick(ctx, time.Now()); got.Delivered != 1 {
		t.Fatalf("scheduler delivered = %d, want 1", got.Delivered)
	}

	// alice has a Sent copy.
	alice, _ := mgr.AuthIMAP("alice@vmail.test", "alicepw")
	if sent := alice.MessagesWithLabel("sent"); len(sent) != 1 {
		t.Errorf("alice Sent = %d, want 1", len(sent))
	}

	// --- 3a. bob reads via IMAP ---
	if n := imapInboxCount(t, imapLn.Addr().String(), "bob@vmail.test", "bobpw"); n != 1 {
		t.Fatalf("bob inbox via IMAP = %d, want 1 (loop delivery broken)", n)
	}

	// --- 3b. bob reads the SAME message via JMAP ---
	subjects := jmapInboxSubjects(t, jmapSrv.URL, "bob@vmail.test", "bobpw")
	if len(subjects) != 1 || subjects[0] != "Lunch?" {
		t.Fatalf("bob inbox via JMAP = %v, want [Lunch?]", subjects)
	}
}

// --- helpers ---

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func smtpSend(t *testing.T, addr, user, pass, from, to, raw string) {
	t.Helper()
	c, err := gosmtp.Dial(addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer c.Close()
	if user != "" {
		if err := c.Auth(sasl.NewPlainClient("", user, pass)); err != nil {
			t.Fatalf("auth: %v", err)
		}
	}
	if err := c.SendMail(from, []string{to}, strings.NewReader(raw)); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func imapInboxCount(t *testing.T, addr, user, pass string) uint32 {
	t.Helper()
	c, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Login(user, pass).Wait(); err != nil {
		t.Fatalf("imap login %s: %v", user, err)
	}
	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatalf("imap select: %v", err)
	}
	_ = c.Logout().Wait()
	return sel.NumMessages
}

func jmapInboxSubjects(t *testing.T, baseURL, user, pass string) []string {
	t.Helper()
	call := func(method string, args map[string]any) map[string]any {
		body, _ := json.Marshal(map[string]any{
			"using":       []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
			"methodCalls": [][]any{{method, args, "0"}},
		})
		req, _ := http.NewRequest("POST", baseURL+"/jmap/api", bytes.NewReader(body))
		req.SetBasicAuth(user, pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			MethodResponses [][]json.RawMessage `json:"methodResponses"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		var res map[string]any
		_ = json.Unmarshal(out.MethodResponses[0][1], &res)
		return res
	}
	q := call("Email/query", map[string]any{"accountId": user, "filter": map[string]any{"inMailbox": "inbox"}})
	var ids []string
	for _, id := range q["ids"].([]any) {
		ids = append(ids, id.(string))
	}
	g := call("Email/get", map[string]any{"accountId": user, "ids": ids})
	var subjects []string
	for _, raw := range g["list"].([]any) {
		subjects = append(subjects, raw.(map[string]any)["subject"].(string))
	}
	return subjects
}
