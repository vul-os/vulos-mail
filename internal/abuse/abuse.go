// Package abuse is the outbound abuse filter — the clean replacement for
// vulos-mail's VulosOutgoingFilter. It protects the shared warm-IP pool from a
// compromised account (ATO): a per-account sliding-window rate limit plus a
// recipient-burst check, escalating to auto-suspend. Reject-only by design — an
// outbound verdict can stop a send but never "deliver then file", so a hijacked
// account can't get the shared IPs blocklisted.
package abuse

import (
	"sync"
	"time"
)

// Action is the outbound disposition (ordered by severity).
type Action int

const (
	Allow    Action = iota
	Throttle        // transient 4xx: slow down
	Block           // permanent 5xx: this message refused
	Suspend         // permanent 5xx: account locked for further sends
)

// Filter enforces per-account outbound limits. Safe for concurrent use.
type Filter struct {
	window        time.Duration
	maxPerWindow  int
	maxRecipients int
	now           func() time.Time

	mu        sync.Mutex
	sends     map[string][]time.Time
	suspended map[string]string // account -> reason
}

// Config configures the filter.
type Config struct {
	Window        time.Duration // sliding window
	MaxPerWindow  int           // max messages per window before throttling
	MaxRecipients int           // recipient count that trips burst auto-suspend
	Now           func() time.Time
}

// New builds a filter with sensible defaults for zero fields.
func New(cfg Config) *Filter {
	if cfg.Window <= 0 {
		cfg.Window = time.Hour
	}
	if cfg.MaxPerWindow <= 0 {
		cfg.MaxPerWindow = 200
	}
	if cfg.MaxRecipients <= 0 {
		cfg.MaxRecipients = 100
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Filter{
		window: cfg.Window, maxPerWindow: cfg.MaxPerWindow, maxRecipients: cfg.MaxRecipients, now: cfg.Now,
		sends: map[string][]time.Time{}, suspended: map[string]string{},
	}
}

// Check evaluates a submission from account to recipientCount recipients and
// records it. Returns the action and a reason.
func (f *Filter) Check(account string, recipientCount int) (Action, string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if reason, ok := f.suspended[account]; ok {
		return Suspend, reason
	}

	// Recipient burst → auto-suspend (classic ATO signal).
	if recipientCount > f.maxRecipients {
		reason := "recipient burst exceeds limit"
		f.suspended[account] = reason
		return Suspend, reason
	}

	now := f.now()
	cutoff := now.Add(-f.window)
	kept := f.sends[account][:0]
	for _, t := range f.sends[account] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	f.sends[account] = kept

	if len(kept) >= f.maxPerWindow {
		return Throttle, "rate limit exceeded"
	}
	f.sends[account] = append(f.sends[account], now)
	return Allow, ""
}

// Suspended reports whether an account is currently suspended.
func (f *Filter) Suspended(account string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.suspended[account]
	return ok
}

// Reinstate clears a suspension (admin action).
func (f *Filter) Reinstate(account string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.suspended, account)
}
