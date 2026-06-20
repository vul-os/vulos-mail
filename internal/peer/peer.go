// Package peer implements vmail's "verified peer" / Vulos-identity badge.
//
// A Registry holds a federation of trusted ("verified") Vulos peer domains.
// A message earns the verified badge only when its authenticated From domain
// belongs to a peer in the registry AND it passed DMARC: membership alone is
// not enough (anyone can claim a domain), and DMARC alone is not enough (it
// authenticates identity but says nothing about trust). Both are required.
package peer

import (
	"strings"
	"sync"
)

// Registry is a concurrency-safe set of trusted Vulos peer domains.
// The zero value is not usable; construct one with NewRegistry.
type Registry struct {
	mu      sync.RWMutex
	domains map[string]struct{}
}

// NewRegistry returns an empty, ready-to-use Registry.
func NewRegistry() *Registry {
	return &Registry{domains: make(map[string]struct{})}
}

// normalize canonicalizes a domain for storage and comparison: it lower-cases
// the value and trims surrounding whitespace and any trailing dot (the root
// label of a fully-qualified domain name).
func normalize(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	return strings.TrimSuffix(d, ".")
}

// Add registers domain as a verified peer. The domain is matched
// case-insensitively. Empty domains are ignored.
func (r *Registry) Add(domain string) {
	d := normalize(domain)
	if d == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains[d] = struct{}{}
}

// AddAll registers each domain in domains as a verified peer.
func (r *Registry) AddAll(domains []string) {
	for _, d := range domains {
		r.Add(d)
	}
}

// IsPeer reports whether domain is a verified peer. Matching is
// case-insensitive, and a subdomain of a peer is itself treated as a peer
// (for example, mail.acme.com matches the peer acme.com).
func (r *Registry) IsPeer(domain string) bool {
	d := normalize(domain)
	if d == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match, then walk up the label hierarchy so a subdomain of a
	// registered peer also matches.
	for {
		if _, ok := r.domains[d]; ok {
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			return false
		}
		d = d[i+1:]
	}
}

// Verified reports whether a message should receive the verified-peer badge.
// It returns true only when fromDomain is a peer (see IsPeer) and dmarcPass is
// true: a verified badge requires both authenticated identity (DMARC) and
// federation membership.
func (r *Registry) Verified(fromDomain string, dmarcPass bool) bool {
	return dmarcPass && r.IsPeer(fromDomain)
}
