package smtpin_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/services/mtaout"
)

func newTestRuntime(t *testing.T) *account.Runtime {
	t.Helper()
	store, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(context.Background(), log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// TestMXRejectsUnknownRecipient is the open-relay regression: the MX hands each
// recipient to Deliver, and Deliver rejects (errors) anyone who is not a local
// account. The sending client must see a failure, so the message is NOT relayed.
func TestMXRejectsUnknownRecipient(t *testing.T) {
	local := map[string]bool{"bob@vulos.to": true}
	var deliveredTo []string
	be := &smtpin.Backend{
		Deliver: func(_ context.Context, rcpt string, _ []byte) error {
			if !local[strings.ToLower(rcpt)] {
				return errors.New("550 no such user; not local")
			}
			deliveredTo = append(deliveredTo, rcpt)
			return nil
		},
	}
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Unauthenticated relay attempt to a NON-local recipient must fail at DATA.
	relayErr := c.SendMail("spammer@evil.example", []string{"victim@elsewhere.example"},
		strings.NewReader("Subject: relay\r\n\r\nplease relay me\r\n"))
	if relayErr == nil {
		t.Fatal("OPEN RELAY: MX accepted mail for a non-local recipient from an unauthenticated sender")
	}
	if len(deliveredTo) != 0 {
		t.Fatalf("OPEN RELAY: message was delivered/relayed: %v", deliveredTo)
	}

	// A local recipient is still accepted (the MX is a real inbound server).
	if err := c.SendMail("someone@out.example", []string{"bob@vulos.to"},
		strings.NewReader("Subject: legit\r\n\r\nhi\r\n")); err != nil {
		t.Fatalf("legitimate local delivery should succeed: %v", err)
	}
	if len(deliveredTo) != 1 || deliveredTo[0] != "bob@vulos.to" {
		t.Fatalf("expected one local delivery, got %v", deliveredTo)
	}
}

// TestSubmissionAuthOverWire verifies, over a real socket, that the MSA refuses
// a wrong password and only enqueues mail after a successful AUTH.
func TestSubmissionAuthOverWire(t *testing.T) {
	rt := newTestRuntime(t)
	var enqueued int
	be := &smtpin.SubmitBackend{
		Auth: func(u, p string) (*account.Runtime, string, error) {
			if u == "alice@vulos.to" && p == "pw" {
				return rt, "vulos.to", nil
			}
			return nil, "", errors.New("invalid credentials")
		},
		Enqueue: func(mtaout.OutMessage) error { enqueued++; return nil },
	}

	// Wrong password must fail AUTH; the message must not be sent.
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewSubmitServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Auth(sasl.NewPlainClient("", "alice@vulos.to", "wrong")); err == nil {
		t.Fatal("submission accepted a wrong password")
	}

	// Correct credentials: AUTH succeeds and the message is enqueued.
	c2, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if err := c2.Auth(sasl.NewPlainClient("", "alice@vulos.to", "pw")); err != nil {
		t.Fatalf("valid AUTH should succeed: %v", err)
	}
	if err := c2.SendMail("alice@vulos.to", []string{"bob@example.com"},
		strings.NewReader("From: alice@vulos.to\r\nSubject: hi\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("authenticated submission should succeed: %v", err)
	}
	if enqueued != 1 {
		t.Fatalf("expected 1 enqueued message after authenticated submission, got %d", enqueued)
	}
}

// TestSubmissionRejectsSenderSpoofing verifies an authenticated user cannot send
// as another address — neither via the envelope MAIL FROM nor the From header
// (the latter is what recipients and DMARC see, so it's the dangerous one).
func TestSubmissionRejectsSenderSpoofing(t *testing.T) {
	rt := newTestRuntime(t)
	be := &smtpin.SubmitBackend{
		Auth: func(u, p string) (*account.Runtime, string, error) {
			if u == "alice@vulos.to" && p == "pw" {
				return rt, "vulos.to", nil
			}
			return nil, "", errors.New("invalid credentials")
		},
		Enqueue: func(mtaout.OutMessage) error { t.Fatal("spoofed message must not be enqueued"); return nil },
	}
	auth := func() gosmtp.Session {
		sess, _ := be.NewSession(nil)
		a, _ := sess.(gosmtp.AuthSession).Auth(sasl.Plain)
		if _, _, err := a.Next([]byte("\x00alice@vulos.to\x00pw")); err != nil {
			t.Fatalf("auth: %v", err)
		}
		return sess
	}

	// 1) Envelope MAIL FROM spoof -> rejected at MAIL.
	if err := auth().Mail("ceo@vulos.to", nil); err == nil {
		t.Error("envelope MAIL FROM spoofing should be rejected")
	}

	// 2) Envelope OK but From: header spoofs another same-domain user -> rejected at DATA.
	s := auth()
	if err := s.Mail("alice@vulos.to", nil); err != nil {
		t.Fatalf("own envelope should be accepted: %v", err)
	}
	_ = s.Rcpt("bob@gmail.com", nil)
	spoof := "From: ceo@vulos.to\r\nTo: bob@gmail.com\r\nSubject: layoffs\r\n\r\nhi\r\n"
	if err := s.Data(strings.NewReader(spoof)); err == nil {
		t.Error("From-header spoofing should be rejected at DATA")
	}
}

// Inbound mail must not be able to forge our Authentication-Results: a pre-existing
// A-R header bearing our authserv-id is stripped before we add our own.
func TestInboundAuthResultsForgeryStripped(t *testing.T) {
	var got []byte
	be := &smtpin.Backend{
		Deliver:    func(_ context.Context, _ string, raw []byte) error { got = raw; return nil },
		AuthServID: "mx.vulos.to",
		Verify:     func([]byte, net.IP, string, string) string { return "dkim=fail" },
	}
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	forged := "From: ceo@vulos.to\r\n" +
		"Authentication-Results: mx.vulos.to; dmarc=pass header.from=ceo@vulos.to\r\n" +
		"Subject: hi\r\n\r\nbody\r\n"
	// recipient must be local-ish; Deliver here accepts anything.
	if err := c.SendMail("attacker@evil.example", []string{"victim@vulos.to"}, strings.NewReader(forged)); err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "dmarc=pass") {
		t.Fatal("forged Authentication-Results (dmarc=pass) survived — auth spoofing possible")
	}
	if !strings.Contains(s, "mx.vulos.to; dkim=fail") {
		t.Errorf("our own Authentication-Results header missing:\n%s", s)
	}
}

// A forged A-R bearing *another* host's authserv-id, plus Received-SPF and a
// foreign ARC chain, must all be stripped from untrusted inbound mail — they
// would otherwise mislead downstream filters/clients that don't pin our id and
// (for ARC) imply a verified chain we never checked.
func TestInboundForeignAuthTraceStripped(t *testing.T) {
	var got []byte
	be := &smtpin.Backend{
		Deliver:    func(_ context.Context, _ string, raw []byte) error { got = raw; return nil },
		AuthServID: "mx.vulos.to",
		Verify:     func([]byte, net.IP, string, string) string { return "dkim=fail" },
	}
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	forged := "From: ceo@vulos.to\r\n" +
		"Authentication-Results: relay.attacker.example; dmarc=pass header.from=ceo@vulos.to\r\n" +
		"Received-SPF: pass (attacker says so) client-ip=1.2.3.4\r\n" +
		"ARC-Seal: i=1; a=rsa-sha256; t=1; cv=none; d=attacker.example; s=s1; b=forged\r\n" +
		"ARC-Message-Signature: i=1; a=rsa-sha256; d=attacker.example; s=s1; b=forged\r\n" +
		"ARC-Authentication-Results: i=1; relay.attacker.example; dmarc=pass\r\n" +
		"Subject: hi\r\n\r\nbody\r\n"
	if err := c.SendMail("attacker@evil.example", []string{"victim@vulos.to"}, strings.NewReader(forged)); err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, bad := range []string{"relay.attacker.example", "Received-SPF:", "ARC-Seal:", "ARC-Message-Signature:", "ARC-Authentication-Results:", "dmarc=pass"} {
		if strings.Contains(s, bad) {
			t.Fatalf("forged auth-trace %q survived stripping:\n%s", bad, s)
		}
	}
	// Our own header is still added, and the body is intact.
	if !strings.Contains(s, "mx.vulos.to; dkim=fail") {
		t.Errorf("our own Authentication-Results header missing:\n%s", s)
	}
	if !strings.Contains(s, "From: ceo@vulos.to") || !strings.HasSuffix(strings.TrimRight(s, "\r\n"), "body") {
		t.Errorf("non-auth headers/body must be preserved:\n%s", s)
	}
}

// With KnownRcpt wired, the MX rejects unknown recipients at RCPT (550 5.1.1)
// rather than accepting the whole message and failing at DATA.
func TestMXRejectsUnknownRcptAtRcptTime(t *testing.T) {
	be := &smtpin.Backend{
		Deliver:   func(context.Context, string, []byte) error { return nil },
		KnownRcpt: func(rcpt string) bool { return rcpt == "bob@vulos.to" },
	}
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail("s@x.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Rcpt("nobody@vulos.to", nil); err == nil {
		t.Error("RCPT to an unknown recipient should be rejected at RCPT time")
	}
	if err := c.Rcpt("bob@vulos.to", nil); err != nil {
		t.Errorf("RCPT to a known recipient should be accepted: %v", err)
	}
}
