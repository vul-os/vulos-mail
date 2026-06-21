package jmap_test

import (
	"bytes"
	"context"
	"net/mail"
	"testing"

	"github.com/vul-os/vulos-mail/internal/model"
)

// A CRLF-laced subject in an Email/set create must not inject extra headers
// (e.g. a hidden Bcc) into the composed message.
func TestJMAPCreateNoHeaderInjection(t *testing.T) {
	ctx := context.Background()
	srv, rt := newServer(t)
	defer srv.Close()

	apiCall(t, srv, "Email/set", map[string]any{
		"accountId": "alice",
		"create": map[string]any{"d1": map[string]any{
			"mailboxIds": map[string]bool{"drafts": true},
			"from":       []map[string]string{{"email": "alice"}},
			"to":         []map[string]string{{"email": "bob@x.test"}},
			"subject":    "hello\r\nBcc: victim@evil.test\r\nX-Injected: yes",
			"textBody":   []map[string]string{{"partId": "t", "type": "text/plain"}},
			"bodyValues": map[string]any{"t": map[string]string{"value": "body"}},
		}},
	})

	drafts := rt.MessagesWithLabel(model.LabelDrafts)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}
	raw, err := rt.Body(ctx, drafts[0].BlobRef)
	if err != nil {
		t.Fatal(err)
	}
	// Parse real header fields: the CRLF must have been encoded into the Subject
	// value (RFC 2047), NOT have created standalone Bcc/X-Injected headers.
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("composed message is unparseable: %v", err)
	}
	if v := msg.Header.Get("Bcc"); v != "" {
		t.Fatalf("Bcc header injected via subject: %q", v)
	}
	if v := msg.Header.Get("X-Injected"); v != "" {
		t.Fatalf("X-Injected header injected via subject: %q", v)
	}
	if got := msg.Header.Get("Subject"); got == "" {
		t.Fatal("subject missing")
	}
}
