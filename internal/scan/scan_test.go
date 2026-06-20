package scan_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/mime"
	"github.com/vul-os/vmail/internal/scan"
)

func multipartWithAttachment(body, attachment string) []byte {
	return []byte("From: a@x.com\r\nTo: b@x.com\r\nSubject: s\r\n" +
		"Content-Type: multipart/mixed; boundary=B\r\n\r\n" +
		"--B\r\nContent-Type: text/plain\r\n\r\n" + body + "\r\n" +
		"--B\r\nContent-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename=\"f.bin\"\r\n\r\n" + attachment + "\r\n" +
		"--B--\r\n")
}

type fakeReporter struct{ reported []string }

func (r *fakeReporter) Report(_ context.Context, h string, _ []byte) error {
	r.reported = append(r.reported, h)
	return nil
}

func TestCSAMMatchRejectsAndReports(t *testing.T) {
	ctx := context.Background()
	raw := multipartWithAttachment("hello", "BAD-PAYLOAD-BYTES")
	atts := mime.ExtractAttachments(raw)
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	sum := sha256.Sum256(atts[0])
	h := hex.EncodeToString(sum[:])

	rep := &fakeReporter{}
	c := scan.NewCSAM(scan.NewSetMatcher(h), rep)
	if v := c.Scan(ctx, raw); v.Action != filter.Reject {
		t.Fatalf("matching attachment should Reject, got %v", v.Action)
	}
	if len(rep.reported) != 1 || rep.reported[0] != h {
		t.Errorf("reporter should have been called with %s, got %v", h, rep.reported)
	}

	// Clean: no matching hash.
	clean := scan.NewCSAM(scan.NewSetMatcher("00deadbeef"), rep)
	if v := clean.Scan(ctx, raw); v.Action != filter.Accept {
		t.Errorf("non-matching attachment should Accept, got %v", v.Action)
	}
}

func TestURLSafety(t *testing.T) {
	ctx := context.Background()
	list := scan.MapBlocklist{
		Malicious:  map[string]bool{"evil.com": true},
		Suspicious: map[string]bool{"sketchy.com": true},
	}
	u := scan.NewURLSafety(list)

	mk := func(body string) []byte {
		return []byte("From: a@x\r\nTo: b@x\r\nSubject: s\r\n\r\n" + body + "\r\n")
	}
	if v := u.Scan(ctx, mk("see http://evil.com/landing now")); v.Action != filter.Reject {
		t.Errorf("malicious URL should Reject, got %v", v.Action)
	}
	if v := u.Scan(ctx, mk("visit http://sketchy.com please")); v.Action != filter.Junk {
		t.Errorf("suspicious URL should Junk, got %v", v.Action)
	}
	if v := u.Scan(ctx, mk("visit http://good.com please")); v.Action != filter.Accept {
		t.Errorf("clean URL should Accept, got %v", v.Action)
	}
}

func TestRspamd(t *testing.T) {
	ctx := context.Background()
	var action string
	var score float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action":"` + action + `","score":` + ftoa(score) + `}`))
	}))
	defer srv.Close()

	r := scan.NewRspamd(srv.URL, 5.0)

	action, score = "reject", 15
	if v := r.Scan(ctx, []byte("Subject: x\r\n\r\nbody")); v.Action != filter.Reject {
		t.Errorf("rspamd reject -> Reject, got %v", v.Action)
	}
	action, score = "add header", 7
	if v := r.Scan(ctx, []byte("Subject: x\r\n\r\nbody")); v.Action != filter.Junk {
		t.Errorf("rspamd add-header -> Junk, got %v", v.Action)
	}
	action, score = "no action", 6 // above JunkScore 5
	if v := r.Scan(ctx, []byte("Subject: x\r\n\r\nbody")); v.Action != filter.Junk {
		t.Errorf("score above threshold -> Junk, got %v", v.Action)
	}
	action, score = "no action", 1
	if v := r.Scan(ctx, []byte("Subject: x\r\n\r\nbody")); v.Action != filter.Accept {
		t.Errorf("clean -> Accept, got %v", v.Action)
	}
}

func ftoa(f float64) string {
	switch f {
	case 15:
		return "15"
	case 7:
		return "7"
	case 6:
		return "6"
	case 1:
		return "1"
	}
	return "0"
}
