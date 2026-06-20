package scan

import (
	"context"
	"regexp"
	"strings"

	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/mime"
)

// Blocklist resolves the safety of a URL (backed by PhishTank/URLhaus/Google
// Safe Browsing feeds in production). Returns whether the URL is malicious
// (reject) and/or suspicious (junk).
type Blocklist interface {
	Lookup(url string) (malicious, suspicious bool)
}

// MapBlocklist is a simple host/url-keyed blocklist.
type MapBlocklist struct {
	Malicious  map[string]bool
	Suspicious map[string]bool
}

func (b MapBlocklist) Lookup(url string) (bool, bool) {
	host := hostOf(url)
	mal := b.Malicious[url] || b.Malicious[host]
	sus := b.Suspicious[url] || b.Suspicious[host]
	return mal, sus
}

var urlRe = regexp.MustCompile(`https?://[^\s"'<>)\]]+`)

// URLSafety extracts URLs from the message body and checks them against a
// blocklist: any malicious URL rejects; otherwise any suspicious URL routes to
// junk.
type URLSafety struct{ List Blocklist }

// NewURLSafety builds a URL-safety scanner.
func NewURLSafety(list Blocklist) *URLSafety { return &URLSafety{List: list} }

func (u *URLSafety) Name() string { return "url-safety" }

func (u *URLSafety) Scan(_ context.Context, raw []byte) filter.Verdict {
	if u.List == nil {
		return filter.Verdict{Action: filter.Accept}
	}
	text, err := mime.ExtractText(raw)
	if err != nil {
		text = string(raw)
	}
	suspicious := false
	for _, url := range urlRe.FindAllString(text, -1) {
		url = strings.TrimRight(url, ".,")
		mal, sus := u.List.Lookup(url)
		if mal {
			return filter.Verdict{Action: filter.Reject, Reason: "malicious-url"}
		}
		if sus {
			suspicious = true
		}
	}
	if suspicious {
		return filter.Verdict{Action: filter.Junk, Reason: "suspicious-url"}
	}
	return filter.Verdict{Action: filter.Accept}
}

func hostOf(url string) string {
	s := url
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}
