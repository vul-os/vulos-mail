package smtpin_test

import (
	"context"
	"net"
	"strings"
	"testing"

	gosmtp "github.com/emersion/go-smtp"

	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
)

// Drive the Session interface directly (no socket) to verify MAIL/RCPT/DATA
// route the raw message to every recipient via Deliver.
func TestSessionDeliversToEachRecipient(t *testing.T) {
	type call struct {
		rcpt string
		raw  string
	}
	var got []call
	be := &smtpin.Backend{
		Deliver: func(_ context.Context, rcpt string, raw []byte) error {
			got = append(got, call{rcpt, string(raw)})
			return nil
		},
	}
	sess, err := be.NewSession(nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := sess.Mail("alice@out.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("bob@vulos.to", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("carol@vulos.to", nil); err != nil {
		t.Fatal(err)
	}
	raw := "Subject: hi\r\n\r\nbody\r\n"
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(got))
	}
	if got[0].rcpt != "bob@vulos.to" || got[1].rcpt != "carol@vulos.to" {
		t.Errorf("recipients wrong: %+v", got)
	}
	if got[0].raw != raw {
		t.Errorf("raw message not passed through verbatim")
	}

	// Reset clears transaction state.
	sess.Reset()
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("after Reset with no rcpts, expected no new deliveries, got %d", len(got))
	}
}

// The MX backend prepends an Authentication-Results header when Verify is set.
func TestMXPrependsAuthResults(t *testing.T) {
	var delivered string
	be := &smtpin.Backend{
		Deliver:    func(_ context.Context, _ string, raw []byte) error { delivered = string(raw); return nil },
		Verify:     func([]byte, net.IP, string, string) string { return "dkim=pass header.d=vulos.to" },
		AuthServID: "mx.vulos.to",
	}
	sess, _ := be.NewSession(nil)
	_ = sess.Mail("a@b.com", nil)
	_ = sess.Rcpt("c@vulos.to", nil)
	if err := sess.Data(strings.NewReader("Subject: x\r\n\r\nbody\r\n")); err != nil {
		t.Fatal(err)
	}
	want := "Authentication-Results: mx.vulos.to; dkim=pass header.d=vulos.to\r\n"
	if !strings.HasPrefix(delivered, want) {
		t.Fatalf("delivered message should start with A-R header; got:\n%q", delivered)
	}
}

// A DMARC p=reject failure is refused with SMTP 550 and never delivered, while
// a fail under p=quarantine/none is still delivered (annotate-only).
func TestMXEnforcesDMARCReject(t *testing.T) {
	delivered := 0
	be := &smtpin.Backend{
		Deliver: func(context.Context, string, []byte) error { delivered++; return nil },
		VerifyVerdict: func([]byte, net.IP, string, string) smtpin.AuthVerdict {
			return smtpin.AuthVerdict{AuthResults: "dmarc=fail header.from=evil.example", Reject: true}
		},
		AuthServID: "mx.vulos.to",
	}
	sess, _ := be.NewSession(nil)
	_ = sess.Mail("a@evil.example", nil)
	_ = sess.Rcpt("c@vulos.to", nil)
	err := sess.Data(strings.NewReader("From: a@evil.example\r\nSubject: x\r\n\r\nbody\r\n"))
	if err == nil {
		t.Fatal("DMARC p=reject failure must be rejected, not delivered")
	}
	if se, ok := err.(*gosmtp.SMTPError); !ok || se.Code != 550 {
		t.Fatalf("want SMTP 550, got %v", err)
	}
	if delivered != 0 {
		t.Fatalf("rejected message must not be delivered (delivered=%d)", delivered)
	}

	// Same fail but Reject=false (quarantine/none) → delivered with the A-R line.
	be.VerifyVerdict = func([]byte, net.IP, string, string) smtpin.AuthVerdict {
		return smtpin.AuthVerdict{AuthResults: "dmarc=fail header.from=evil.example", Reject: false}
	}
	sess2, _ := be.NewSession(nil)
	_ = sess2.Mail("a@evil.example", nil)
	_ = sess2.Rcpt("c@vulos.to", nil)
	if err := sess2.Data(strings.NewReader("From: a@evil.example\r\nSubject: x\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("quarantine/none fail should still deliver: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("annotate-only fail should be delivered once, got %d", delivered)
	}
}
