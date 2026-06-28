// Package region maps a mailbox to the cell that owns its data.
//
// Phase-0 hook: single-cell (eu). The Resolver is a stub that always returns
// EU because there is only one cell today. Later phases add per-mailbox or
// per-domain config entries so a mailbox homed on a different cell can be
// reached by a router/proxy — this package is the seam for that expansion.
//
// No behavior changes are introduced: callers that ignore the resolver see the
// same local store they always have; the resolver merely annotates a mailbox
// with the region that owns it.
package region

import (
	"strings"
	"sync"
)

// Region is an opaque cell identifier. The only value wired in Phase-0 is EU.
type Region string

const (
	// EU is the sole active region in Phase-0; all mailboxes are homed here.
	EU Region = "eu"

	// Default is the fallback region returned when no explicit home is configured.
	Default Region = EU
)

// Endpoint is the resolved location of a mailbox's mail storage.
type Endpoint struct {
	Region Region
	// URL is the internal base URL of the cell. Empty in Phase-0 (single-cell):
	// callers use the local store directly rather than a remote endpoint.
	URL string
}

// Resolver maps a mailbox address to the cell that owns its data.
// The zero value is usable: all mailboxes resolve to EU.
//
// Phase-0 stub: only EU is configured; URL is always empty (local store).
// Future phases call SetMailbox / SetDomain to pin mailboxes to remote cells.
type Resolver struct {
	mu     sync.RWMutex
	byAddr map[string]Endpoint // exact address overrides (lower-cased)
	byDom  map[string]Endpoint // per-domain overrides (lower-cased)
}

// New returns an empty Resolver in which all mailboxes resolve to EU.
func New() *Resolver {
	return &Resolver{
		byAddr: map[string]Endpoint{},
		byDom:  map[string]Endpoint{},
	}
}

// SetMailbox pins a specific mailbox address to a region and endpoint URL.
// Intended for future cross-cell configuration; unused in Phase-0.
func (r *Resolver) SetMailbox(address, rgn, url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byAddr[strings.ToLower(address)] = Endpoint{Region: Region(rgn), URL: url}
}

// SetDomain pins every mailbox in domain to a region and endpoint URL.
// Per-mailbox overrides (SetMailbox) take precedence over domain overrides.
// Intended for future cross-cell configuration; unused in Phase-0.
func (r *Resolver) SetDomain(domain, rgn, url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byDom[strings.ToLower(domain)] = Endpoint{Region: Region(rgn), URL: url}
}

// Resolve returns the cell Endpoint for mailbox.
//
// Phase-0: always returns {EU, ""} (empty URL means: use the local store).
// When cross-cell config is added, per-address entries take precedence over
// per-domain entries; both fall back to EU.
//
// Resolve is safe to call on a nil Resolver: it returns the EU default.
func (r *Resolver) Resolve(mailbox string) Endpoint {
	if r == nil {
		return Endpoint{Region: Default}
	}
	mailbox = strings.ToLower(mailbox)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ep, ok := r.byAddr[mailbox]; ok {
		return ep
	}
	if i := strings.LastIndex(mailbox, "@"); i >= 0 {
		if ep, ok := r.byDom[mailbox[i+1:]]; ok {
			return ep
		}
	}
	return Endpoint{Region: Default}
}
