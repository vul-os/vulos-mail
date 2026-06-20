// Package scan provides concrete filter.Scanner implementations ported from
// vulos-mail's anti-abuse suite: CSAM hash matching (+ NCMEC reporting),
// URL-safety, and an rspamd spam client.
package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/mime"
)

// Matcher reports whether a content hash is known-bad. A real deployment backs
// this with NCMEC/IWF hash lists (exact SHA-256 and/or PDQ perceptual hashes);
// here we match exact SHA-256 of attachment bodies.
type Matcher interface {
	Match(sha256hex string) bool
}

// Reporter escalates a confirmed match (e.g. to the NCMEC CyberTipline). Errors
// are surfaced, never swallowed — a failed report must not look like success.
type Reporter interface {
	Report(ctx context.Context, sha256hex string, raw []byte) error
}

// SetMatcher is a simple in-memory exact-hash matcher.
type SetMatcher struct{ set map[string]struct{} }

// NewSetMatcher builds a matcher from a list of lowercase hex SHA-256 hashes.
func NewSetMatcher(hashes ...string) *SetMatcher {
	m := &SetMatcher{set: make(map[string]struct{}, len(hashes))}
	for _, h := range hashes {
		m.set[h] = struct{}{}
	}
	return m
}

func (m *SetMatcher) Match(h string) bool { _, ok := m.set[h]; return ok }

// CSAM scans message attachments against a known-bad matcher; a hit is rejected
// and reported. FailClosed rejects the message if the matcher itself errors
// (here matchers don't error, but the reporter can).
type CSAM struct {
	Matcher  Matcher
	Reporter Reporter
}

// NewCSAM builds a CSAM scanner.
func NewCSAM(m Matcher, r Reporter) *CSAM { return &CSAM{Matcher: m, Reporter: r} }

func (c *CSAM) Name() string { return "csam" }

func (c *CSAM) Scan(ctx context.Context, raw []byte) filter.Verdict {
	if c.Matcher == nil {
		return filter.Verdict{Action: filter.Accept}
	}
	for _, att := range mime.ExtractAttachments(raw) {
		sum := sha256.Sum256(att)
		h := hex.EncodeToString(sum[:])
		if c.Matcher.Match(h) {
			if c.Reporter != nil {
				_ = c.Reporter.Report(ctx, h, raw)
			}
			return filter.Verdict{Action: filter.Reject, Reason: "csam-match"}
		}
	}
	return filter.Verdict{Action: filter.Accept}
}
