package compose_test

import (
	"strings"
	"testing"

	"github.com/vul-os/vmail/internal/compose"
	"github.com/vul-os/vmail/internal/mime"
)

func TestBuildWithAttachmentRoundTrips(t *testing.T) {
	raw, err := compose.Build(compose.Message{
		From: "alice@vmail.test", To: []string{"Bob <bob@x.com>"}, Cc: []string{"c@x.com"},
		Subject: "Hi", Text: "plain body", HTML: "<p>rich <b>body</b></p>",
		MessageID:   "m1@vmail.test",
		Attachments: []compose.Attachment{{Name: "note.txt", Type: "text/plain", Data: []byte("file-bytes")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	env, err := mime.ParseEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if env.Subject != "Hi" || len(env.From) == 0 || env.From[0] != "alice@vmail.test" {
		t.Errorf("envelope wrong: %+v", env)
	}
	if len(env.To) != 1 || env.To[0] != "bob@x.com" {
		t.Errorf("To = %v", env.To)
	}

	text, _ := mime.ExtractText(raw)
	if !strings.Contains(text, "plain body") || !strings.Contains(text, "rich") {
		t.Errorf("body text missing parts: %q", text)
	}

	atts := mime.Attachments(raw)
	if len(atts) != 1 || atts[0].Name != "note.txt" || string(atts[0].Data) != "file-bytes" {
		t.Errorf("attachment round-trip failed: %+v", atts)
	}
}

func TestBuildTextOnly(t *testing.T) {
	raw, err := compose.Build(compose.Message{From: "a@x", To: []string{"b@x"}, Subject: "s", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if txt, _ := mime.ExtractText(raw); !strings.Contains(txt, "hello") {
		t.Errorf("text body missing: %q", txt)
	}
}
