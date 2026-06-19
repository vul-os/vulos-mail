package smtpin_test

import (
	"context"
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
