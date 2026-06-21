package smtpin_test

import (
	"context"
	"fmt"
	"testing"

	gosmtp "github.com/emersion/go-smtp"

	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
)

// The MX must cap recipients per transaction (anti-amplification / DoS). go-smtp
// is configured with MaxRecipients=100; beyond that, RCPT must be refused.
func TestMXEnforcesRecipientLimit(t *testing.T) {
	be := &smtpin.Backend{
		Deliver:   func(context.Context, string, []byte) error { return nil },
		KnownRcpt: func(string) bool { return true },
	}
	ln := listenTCP(t)
	defer ln.Close()
	go smtpin.NewServer(be, "", "vulos.to").Serve(ln)

	c, err := gosmtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail("s@out.example", nil); err != nil {
		t.Fatal(err)
	}
	refusedAt := 0
	for i := 0; i < 200; i++ {
		if err := c.Rcpt(fmt.Sprintf("r%d@out.example", i), nil); err != nil {
			refusedAt = i
			break
		}
	}
	if refusedAt == 0 {
		t.Fatal("MX accepted 200 recipients — MaxRecipients not enforced (DoS/amplification risk)")
	}
	if refusedAt > 100 {
		t.Fatalf("recipient cap too high: refused at %d, want <=100", refusedAt)
	}
}
