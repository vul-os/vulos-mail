package filter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vul-os/vmail/internal/filter"
)

// keywordScanner flags messages containing a substring.
type keywordScanner struct {
	name   string
	needle string
	action filter.Action
}

func (k keywordScanner) Name() string { return k.name }
func (k keywordScanner) Scan(_ context.Context, raw []byte) filter.Verdict {
	if strings.Contains(string(raw), k.needle) {
		return filter.Verdict{Action: k.action, Reason: k.name}
	}
	return filter.Verdict{Action: filter.Accept}
}

func TestChainFoldsToWorstAndShortCircuitsReject(t *testing.T) {
	ctx := context.Background()
	c := filter.NewChain(
		keywordScanner{"spammy", "VIAGRA", filter.Junk},
		keywordScanner{"malware", "evil.exe", filter.Reject},
	)

	if v := c.Scan(ctx, []byte("hello friend")); v.Action != filter.Accept {
		t.Errorf("clean message = %v, want Accept", v.Action)
	}
	if v := c.Scan(ctx, []byte("buy VIAGRA now")); v.Action != filter.Junk || v.Reason != "spammy" {
		t.Errorf("spammy = %+v, want Junk/spammy", v)
	}
	// Reject wins even alongside junk.
	if v := c.Scan(ctx, []byte("VIAGRA and evil.exe")); v.Action != filter.Reject || v.Reason != "malware" {
		t.Errorf("malware = %+v, want Reject/malware", v)
	}
}
