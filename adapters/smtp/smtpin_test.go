package smtpin_test

import (
	"context"
	"net"
	"strings"
	"testing"

	smtpin "github.com/vul-os/vmail/adapters/smtp"
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
	if err := sess.Rcpt("bob@vmail.test", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.Rcpt("carol@vmail.test", nil); err != nil {
		t.Fatal(err)
	}
	raw := "Subject: hi\r\n\r\nbody\r\n"
	if err := sess.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(got))
	}
	if got[0].rcpt != "bob@vmail.test" || got[1].rcpt != "carol@vmail.test" {
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
		Verify:     func([]byte, net.IP, string, string) string { return "dkim=pass header.d=vmail.test" },
		AuthServID: "mx.vmail.test",
	}
	sess, _ := be.NewSession(nil)
	_ = sess.Mail("a@b.com", nil)
	_ = sess.Rcpt("c@vmail.test", nil)
	if err := sess.Data(strings.NewReader("Subject: x\r\n\r\nbody\r\n")); err != nil {
		t.Fatal(err)
	}
	want := "Authentication-Results: mx.vmail.test; dkim=pass header.d=vmail.test\r\n"
	if !strings.HasPrefix(delivered, want) {
		t.Fatalf("delivered message should start with A-R header; got:\n%q", delivered)
	}
}
