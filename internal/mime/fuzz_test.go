package mime_test

import (
	"testing"

	"github.com/vul-os/vulos-mail/internal/mime"
)

// Parsing arbitrary/hostile bytes must never panic (the MX feeds untrusted mail
// straight into these).
func FuzzParse(f *testing.F) {
	f.Add([]byte("From: a@b\r\nTo: c@d\r\nSubject: x\r\n\r\nbody\r\n"))
	f.Add([]byte("Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\n\r\nx\r\n--b--\r\n"))
	f.Add([]byte(""))
	f.Add([]byte("\r\n\r\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = mime.ParseEnvelope(raw)
		_, _ = mime.ExtractText(raw)
		_ = mime.Attachments(raw)
		_ = mime.ExtractAttachments(raw)
	})
}
