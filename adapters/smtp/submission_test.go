package smtpin_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/authlimit"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/dkim"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/services/mtaout"
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
			if u == "alice@vulos.to" && p == "secret" {
				return rt, "tenant-a", nil
			}
			return nil, "", gosmtp.ErrAuthFailed
		},
		Enqueue: func(m mtaout.OutMessage) error { enqueued = append(enqueued, m); return nil },
	}
	srv := smtpin.NewSubmitServer(be, "127.0.0.1:0", "vulos.to")
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
	if err := c.Auth(sasl.NewPlainClient("", "alice@vulos.to", "secret")); err != nil {
		t.Fatalf("auth: %v", err)
	}

	raw := []byte("From: alice@vulos.to\r\nTo: bob@gmail.com, carol@yahoo.com\r\nSubject: Hi\r\n\r\nbody\r\n")
	if err := c.Mail("alice@vulos.to", nil); err != nil {
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
		if m.Tenant != "tenant-a" || m.FromDomain != "vulos.to" {
			t.Errorf("bad OutMessage fields: %+v", m)
		}
	}
	if !domains["gmail.com"] || !domains["yahoo.com"] {
		t.Errorf("destination domains = %v", domains)
	}
}

// TestSubmissionBruteForceThrottled proves repeated failed AUTH PLAIN attempts
// lock the source out (even a subsequent correct password fails), and that the
// correct password works again once the lockout window elapses.
func TestSubmissionBruteForceThrottled(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, _ := account.Open(ctx, log, store, ids.NewGen(), nil)

	clock := time.Unix(0, 0).UTC()
	lim := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: 10 * time.Minute, Now: func() time.Time { return clock }})

	be := &smtpin.SubmitBackend{
		Auth: func(u, p string) (*account.Runtime, string, error) {
			if u == "alice@vulos.to" && p == "secret" {
				return rt, "tenant-a", nil
			}
			return nil, "", gosmtp.ErrAuthFailed
		},
		Enqueue: func(mtaout.OutMessage) error { return nil },
		Limiter: lim,
	}
	srv := smtpin.NewSubmitServer(be, "127.0.0.1:0", "vulos.to")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	auth := func(pass string) error {
		c, err := gosmtp.Dial(ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.Auth(sasl.NewPlainClient("", "alice@vulos.to", pass))
	}

	for i := 0; i < 3; i++ {
		if err := auth("wrong"); err == nil {
			t.Fatalf("attempt %d: wrong password should fail", i)
		}
	}
	// Locked: even the correct password is refused.
	if err := auth("secret"); err == nil {
		t.Fatal("correct password should be refused after lockout")
	}
	// Lockout elapses → correct password works again.
	clock = clock.Add(11 * time.Minute)
	if err := auth("secret"); err != nil {
		t.Fatalf("correct password should work after lockout expires: %v", err)
	}
}

// Submitted mail is DKIM-signed with the From domain's key and verifies.
func TestSubmissionDKIMSigns(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, _ := account.Open(ctx, log, store, ids.NewGen(), nil)

	key, txt, err := dkim.GenerateRSAKey(1024)
	if err != nil {
		t.Fatal(err)
	}
	signer := dkim.NewSigner()
	signer.AddDomain("vulos.to", "s1", key)

	var enqueued []mtaout.OutMessage
	be := &smtpin.SubmitBackend{
		Auth:    func(string, string) (*account.Runtime, string, error) { return rt, "t", nil },
		Enqueue: func(m mtaout.OutMessage) error { enqueued = append(enqueued, m); return nil },
		Signer:  signer,
	}
	sess, _ := be.NewSession(nil)
	// Authenticate via the SASL PLAIN server (sets the runtime on the session).
	authSrv, err := sess.(gosmtp.AuthSession).Auth(sasl.Plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := authSrv.Next([]byte("\x00alice@vulos.to\x00pw")); err != nil { // authzid\0authcid\0passwd
		t.Fatalf("sasl: %v", err)
	}

	if err := sess.Mail("alice@vulos.to", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("bob@gmail.com", nil); err != nil {
		t.Fatal(err)
	}
	raw := "From: alice@vulos.to\r\nTo: bob@gmail.com\r\nSubject: Signed\r\n\r\nhello\r\n"
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 enqueued, got %d", len(enqueued))
	}
	results, err := dkim.Verify(enqueued[0].Raw, func(d string) ([]string, error) {
		if d == "s1._domainkey.vulos.to" {
			return []string{txt}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dkim.Aligned(results, "vulos.to") {
		t.Errorf("submitted mail should carry an aligned DKIM signature, got %+v", results)
	}
}

// Submission without auth is rejected.
func TestSubmissionRequiresAuth(t *testing.T) {
	be := &smtpin.SubmitBackend{
		Auth:    func(string, string) (*account.Runtime, string, error) { return nil, "", gosmtp.ErrAuthFailed },
		Enqueue: func(mtaout.OutMessage) error { return nil },
	}
	sess, _ := be.NewSession(nil)
	if err := sess.Mail("x@y.com", nil); err == nil {
		t.Error("MAIL before auth should fail")
	}
}
