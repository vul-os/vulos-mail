package mime_test

import (
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/mime"
)

// An attachment nested inside an inner multipart must still be extracted (the
// CSAM hash scan feeds off ExtractAttachments/Attachments, so missing a nested
// part would be an evasion).
func TestNestedAttachmentExtracted(t *testing.T) {
	raw := strings.ReplaceAll(`From: a@b
To: c@d
Subject: nested
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="OUT"

--OUT
Content-Type: multipart/alternative; boundary="IN"

--IN
Content-Type: text/plain

hello
--IN
Content-Type: text/html

<p>hello</p>
--IN--
--OUT
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="evil.bin"

SECRETPAYLOAD
--OUT--
`, "\n", "\r\n")

	atts := mime.Attachments([]byte(raw))
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1 (nested attachment missed = scan evasion)", len(atts))
	}
	if atts[0].Name != "evil.bin" || !strings.Contains(string(atts[0].Data), "SECRETPAYLOAD") {
		t.Errorf("nested attachment wrong: %+v", atts[0])
	}
	if txt, _ := mime.ExtractText([]byte(raw)); !strings.Contains(txt, "hello") {
		t.Errorf("nested inline text missed: %q", txt)
	}
}
