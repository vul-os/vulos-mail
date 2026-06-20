package mime_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/compose"
	"github.com/vul-os/vmail/internal/mime"
)

func TestParseEnvelopeEncodedAndRefs(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: bob@example.com, carol@example.com\r\n" +
		"Subject: =?utf-8?q?caf=C3=A9_meeting?=\r\n" +
		"Message-ID: <m1@example.com>\r\n" +
		"In-Reply-To: <m0@example.com>\r\n" +
		"References: <root@example.com> <m0@example.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\nhello\r\n"

	env, err := mime.ParseEnvelope([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if env.Subject != "café meeting" {
		t.Errorf("encoded subject not decoded: %q", env.Subject)
	}
	if len(env.From) != 1 || env.From[0] != "alice@example.com" {
		t.Errorf("From = %v", env.From)
	}
	if len(env.To) != 2 {
		t.Errorf("To = %v, want 2", env.To)
	}
	if env.MessageIDHeader != "m1@example.com" {
		t.Errorf("Message-ID = %q", env.MessageIDHeader)
	}
	if env.InReplyTo != "m0@example.com" {
		t.Errorf("In-Reply-To = %q", env.InReplyTo)
	}
	if len(env.References) != 2 || env.References[1] != "m0@example.com" {
		t.Errorf("References = %v", env.References)
	}
}

func TestExtractTextMultipart(t *testing.T) {
	raw := "From: a@x\r\nTo: b@x\r\nSubject: s\r\n" +
		"Content-Type: multipart/alternative; boundary=BOUND\r\n\r\n" +
		"--BOUND\r\nContent-Type: text/plain\r\n\r\nplain body words\r\n" +
		"--BOUND\r\nContent-Type: text/html\r\n\r\n<p>html body</p>\r\n" +
		"--BOUND--\r\n"

	text, err := mime.ExtractText([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "plain body words") {
		t.Errorf("plain part missing from extracted text: %q", text)
	}
}

// TestParseEnvelopeFromName covers the display-name extraction the dashboard
// relies on: From: "Fly.io" <billing@fly.io> -> FromName "Fly.io".
func TestParseEnvelopeFromName(t *testing.T) {
	raw := "From: \"Fly.io\" <billing@fly.io>\r\n" +
		"To: user@example.com\r\n" +
		"Cc: ops@example.com, finance@example.com\r\n" +
		"Subject: Your invoice\r\n" +
		"Date: Tue, 03 Jan 2006 15:04:05 -0700\r\n" +
		"Message-ID: <inv-1@fly.io>\r\n" +
		"References: <a@fly.io> <b@fly.io>\r\n" +
		"\r\nbody\r\n"

	env, err := mime.ParseEnvelope([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if env.FromName != "Fly.io" {
		t.Errorf("FromName = %q, want %q", env.FromName, "Fly.io")
	}
	if len(env.From) != 1 || env.From[0] != "billing@fly.io" {
		t.Errorf("From = %v, want [billing@fly.io]", env.From)
	}
	if len(env.To) != 1 || env.To[0] != "user@example.com" {
		t.Errorf("To = %v", env.To)
	}
	if len(env.Cc) != 2 || env.Cc[0] != "ops@example.com" || env.Cc[1] != "finance@example.com" {
		t.Errorf("Cc = %v", env.Cc)
	}
	if env.Subject != "Your invoice" {
		t.Errorf("Subject = %q", env.Subject)
	}
	if env.Date.IsZero() {
		t.Errorf("Date not parsed")
	}
	if env.Date.Location() != time.UTC {
		t.Errorf("Date not normalized to UTC: %v", env.Date)
	}
	if env.MessageIDHeader != "inv-1@fly.io" {
		t.Errorf("Message-ID = %q", env.MessageIDHeader)
	}
	if len(env.References) != 2 || env.References[0] != "a@fly.io" {
		t.Errorf("References = %v", env.References)
	}
}

// TestParseEnvelopePlainAddress: a bare address with no display name leaves
// FromName empty.
func TestParseEnvelopePlainAddress(t *testing.T) {
	raw := "From: billing@fly.io\r\n" +
		"To: user@example.com\r\n" +
		"Subject: s\r\n" +
		"\r\nbody\r\n"

	env, err := mime.ParseEnvelope([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if env.FromName != "" {
		t.Errorf("FromName = %q, want empty for bare address", env.FromName)
	}
	if len(env.From) != 1 || env.From[0] != "billing@fly.io" {
		t.Errorf("From = %v", env.From)
	}
}

// TestParseEnvelopeMalformed: unparseable input returns an error, no panic.
func TestParseEnvelopeMalformed(t *testing.T) {
	if _, err := mime.ParseEnvelope([]byte("\x00\x01not a message")); err == nil {
		t.Errorf("expected error for malformed input")
	}
}

// TestParseEnvelopeEmpty: empty input does not panic; returns error or zero value.
func TestParseEnvelopeEmpty(t *testing.T) {
	env, err := mime.ParseEnvelope(nil)
	if err == nil && (env.Subject != "" || len(env.From) != 0) {
		t.Errorf("unexpected envelope for empty input: %+v", env)
	}
}

// TestExtractTextPlain: simple text/plain message returns its body.
func TestExtractTextPlain(t *testing.T) {
	raw := "From: a@x\r\nTo: b@x\r\nSubject: s\r\n" +
		"Content-Type: text/plain\r\n\r\nhello world\r\n"

	text, err := mime.ExtractText([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "hello world") {
		t.Errorf("plain body missing: %q", text)
	}
}

// TestExtractTextHTMLOnly: an HTML-only message still yields the HTML as inline
// text (ExtractText returns the raw HTML for now).
func TestExtractTextHTMLOnly(t *testing.T) {
	raw := "From: a@x\r\nTo: b@x\r\nSubject: s\r\n" +
		"Content-Type: text/html\r\n\r\n<p>only <b>html</b></p>\r\n"

	text, err := mime.ExtractText([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "html") {
		t.Errorf("html body missing from extracted text: %q", text)
	}
}

// TestExtractTextMalformed: bad input does not panic.
func TestExtractTextMalformed(t *testing.T) {
	if _, err := mime.ExtractText([]byte("\x00garbage")); err == nil {
		t.Errorf("expected error for malformed input")
	}
}

// buildMixed constructs a multipart/mixed message (text + two attachments).
func buildMixed(t *testing.T) []byte {
	t.Helper()
	raw, err := compose.Build(compose.Message{
		From:    "alice@vmail.test",
		To:      []string{"bob@x.com"},
		Subject: "with files",
		Text:    "see attached",
		Attachments: []compose.Attachment{
			{Name: "note.txt", Type: "text/plain", Data: []byte("first attachment")},
			{Name: "data.csv", Type: "text/csv", Data: []byte("a,b,c\n1,2,3\n")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestAttachmentsRoundTrip: count, names, content types, and bytes survive.
func TestAttachmentsRoundTrip(t *testing.T) {
	raw := buildMixed(t)

	atts := mime.Attachments(raw)
	if len(atts) != 2 {
		t.Fatalf("got %d attachments, want 2", len(atts))
	}
	if atts[0].Name != "note.txt" {
		t.Errorf("att[0].Name = %q", atts[0].Name)
	}
	if !strings.HasPrefix(atts[0].Type, "text/plain") {
		t.Errorf("att[0].Type = %q", atts[0].Type)
	}
	if string(atts[0].Data) != "first attachment" {
		t.Errorf("att[0].Data = %q", atts[0].Data)
	}
	if atts[1].Name != "data.csv" {
		t.Errorf("att[1].Name = %q", atts[1].Name)
	}
	if !strings.HasPrefix(atts[1].Type, "text/csv") {
		t.Errorf("att[1].Type = %q", atts[1].Type)
	}
	if string(atts[1].Data) != "a,b,c\n1,2,3\n" {
		t.Errorf("att[1].Data = %q", atts[1].Data)
	}
}

// TestExtractAttachments returns just the raw bodies in order.
func TestExtractAttachments(t *testing.T) {
	raw := buildMixed(t)
	bodies := mime.ExtractAttachments(raw)
	if len(bodies) != 2 {
		t.Fatalf("got %d bodies, want 2", len(bodies))
	}
	if string(bodies[0]) != "first attachment" {
		t.Errorf("bodies[0] = %q", bodies[0])
	}
	if string(bodies[1]) != "a,b,c\n1,2,3\n" {
		t.Errorf("bodies[1] = %q", bodies[1])
	}
}

// TestExtractAttachmentsNone: a message with no attachments yields nil/empty.
func TestExtractAttachmentsNone(t *testing.T) {
	raw := "From: a@x\r\nTo: b@x\r\nSubject: s\r\n" +
		"Content-Type: text/plain\r\n\r\njust text\r\n"
	if got := mime.ExtractAttachments([]byte(raw)); len(got) != 0 {
		t.Errorf("expected no attachments, got %v", got)
	}
	if got := mime.Attachments([]byte(raw)); len(got) != 0 {
		t.Errorf("expected no attachments, got %v", got)
	}
}

// TestExtractAttachmentsMalformed: bad input returns nil, no panic.
func TestExtractAttachmentsMalformed(t *testing.T) {
	if got := mime.ExtractAttachments([]byte("\x00bad")); got != nil {
		t.Errorf("expected nil for malformed input, got %v", got)
	}
	if got := mime.Attachments([]byte("\x00bad")); got != nil {
		t.Errorf("expected nil for malformed input, got %v", got)
	}
}

// TestAttachmentAt: in-range hits and out-of-range bounds.
func TestAttachmentAt(t *testing.T) {
	raw := buildMixed(t)

	a0, ok := mime.AttachmentAt(raw, 0)
	if !ok || a0.Name != "note.txt" {
		t.Errorf("AttachmentAt(0) = %+v, ok=%v", a0, ok)
	}
	a1, ok := mime.AttachmentAt(raw, 1)
	if !ok || a1.Name != "data.csv" {
		t.Errorf("AttachmentAt(1) = %+v, ok=%v", a1, ok)
	}

	if a, ok := mime.AttachmentAt(raw, 2); ok || a.Name != "" {
		t.Errorf("AttachmentAt(2) should be out of range, got %+v ok=%v", a, ok)
	}
	if a, ok := mime.AttachmentAt(raw, -1); ok || a.Name != "" {
		t.Errorf("AttachmentAt(-1) should be out of range, got %+v ok=%v", a, ok)
	}
}
