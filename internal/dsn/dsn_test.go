package dsn_test

import (
	"bytes"
	"io"
	"net/mail"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/dsn"
)

func TestBuildBounceStructure(t *testing.T) {
	reportingDomain := "vulos.to"
	sender := "alice@example.com"
	recipients := []string{"bob@gone.example", "carol@nowhere.example"}
	reason := "550 5.1.1 user unknown"

	raw := dsn.Build(reportingDomain, sender, recipients, reason)
	if len(raw) == 0 {
		t.Fatal("Build returned empty message")
	}

	// Must parse as a well-formed RFC 5322 message.
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Build output is not a parseable message: %v", err)
	}

	// From must look like a mailer-daemon address at the reporting domain.
	from, err := msg.Header.AddressList("From")
	if err != nil {
		t.Fatalf("parsing From: %v", err)
	}
	if len(from) != 1 {
		t.Fatalf("From address count = %d, want 1", len(from))
	}
	fromAddr := strings.ToLower(from[0].Address)
	if !strings.HasPrefix(fromAddr, "mailer-daemon@") {
		t.Errorf("From = %q, want a MAILER-DAEMON address", fromAddr)
	}
	if !strings.HasSuffix(fromAddr, "@"+reportingDomain) {
		t.Errorf("From = %q, want it at reporting domain %q", fromAddr, reportingDomain)
	}

	// To must be the original sender.
	to, err := msg.Header.AddressList("To")
	if err != nil {
		t.Fatalf("parsing To: %v", err)
	}
	if len(to) != 1 || !strings.EqualFold(to[0].Address, sender) {
		t.Errorf("To = %+v, want original sender %q", to, sender)
	}

	// Should be auto-submitted so it doesn't trigger loops.
	if got := msg.Header.Get("Auto-Submitted"); got == "" {
		t.Error("missing Auto-Submitted header on bounce")
	}
	if subj := msg.Header.Get("Subject"); subj == "" {
		t.Error("missing Subject header on bounce")
	}

	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	body := string(bodyBytes)

	// The body must name every failed recipient and the reason.
	for _, r := range recipients {
		if !strings.Contains(body, r) {
			t.Errorf("body missing failed recipient %q\nbody:\n%s", r, body)
		}
	}
	if !strings.Contains(body, reason) {
		t.Errorf("body missing reason %q\nbody:\n%s", reason, body)
	}
}

func TestBuildSingleRecipient(t *testing.T) {
	raw := dsn.Build("mx.vulos.to", "sender@origin.example", []string{"target@dead.example"}, "timed out")

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not parseable: %v", err)
	}
	body, _ := io.ReadAll(msg.Body)
	if !strings.Contains(string(body), "target@dead.example") {
		t.Error("single recipient not present in body")
	}
	if !strings.Contains(string(body), "timed out") {
		t.Error("reason not present in body")
	}
}

// CRLF line endings are required for SMTP/RFC 5322 wire format.
func TestBuildUsesCRLF(t *testing.T) {
	raw := dsn.Build("vulos.to", "s@x.example", []string{"r@y.example"}, "nope")
	if !bytes.Contains(raw, []byte("\r\n")) {
		t.Fatal("bounce does not use CRLF line endings")
	}
	// Header/body separator must be a blank CRLF line.
	if !bytes.Contains(raw, []byte("\r\n\r\n")) {
		t.Error("missing CRLF header/body separator")
	}
}

// The bounce is a proper multipart/report (RFC 3464) with a machine-readable
// delivery-status part, and injected reasons/recipients can't break out.
func TestBuildMultipartReport(t *testing.T) {
	raw := dsn.Build("vulos.to", "alice@x.com", []string{"bob@gone.example"},
		"550 boom\r\nInjected: evil")
	s := string(raw)
	if !strings.Contains(s, "multipart/report; report-type=delivery-status") {
		t.Error("not a multipart/report DSN")
	}
	if !strings.Contains(s, "message/delivery-status") {
		t.Error("missing message/delivery-status part")
	}
	if !strings.Contains(s, "Final-Recipient: rfc822; bob@gone.example") {
		t.Error("missing Final-Recipient in delivery-status")
	}
	if !strings.Contains(s, "Action: failed") || !strings.Contains(s, "Status: 5.0.0") {
		t.Error("missing Action/Status in delivery-status")
	}
	// CRLF injection in the reason must be neutralized (no new header line).
	if strings.Contains(s, "\r\nInjected: evil") {
		t.Error("CRLF injection via reason was not sanitized")
	}
}
