package mime_test

import (
	"strings"
	"testing"

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
