package dsn_test

import (
	"bytes"
	"net/mail"
	"testing"

	"github.com/vul-os/vulos-mail/internal/dsn"
)

// A DSN built from arbitrary (possibly injection-laced) inputs must always be a
// parseable message — i.e. CRLF injection can never forge headers/parts.
func FuzzBuild(f *testing.F) {
	f.Add("vulos.to", "a@b", "c@d", "550 nope")
	f.Add("x", "s@x", "r@y", "boom\r\nInjected: evil")
	f.Fuzz(func(t *testing.T, dom, sender, rcpt, reason string) {
		raw := dsn.Build(dom, sender, []string{rcpt}, reason)
		if _, err := mail.ReadMessage(bytes.NewReader(raw)); err != nil {
			t.Fatalf("DSN not parseable for inputs (%q,%q,%q,%q): %v", dom, sender, rcpt, reason, err)
		}
	})
}
