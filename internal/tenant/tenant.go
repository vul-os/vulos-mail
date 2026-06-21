// Package tenant provides multi-tenancy: a registry mapping domains to tenant
// ids, and per-tenant daily quotas (message + byte caps). Ported from
// vulos-mail's tenant registry + tenantquota. A tenant groups accounts so a
// customer's aggregate usage and reputation are tracked together.
package tenant

import (
	"strings"
	"sync"
	"time"
)

// Registry maps an account/domain to a stable tenant id. Unmapped domains are
// their own tenant (the domain string), so single-tenant setups need no config.
type Registry struct {
	mu       sync.RWMutex
	byDomain map[string]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{byDomain: map[string]string{}} }

// Map assigns a domain to a tenant id.
func (r *Registry) Map(domain, tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byDomain[strings.ToLower(domain)] = tenantID
}

// TenantFor resolves an address (or domain) to its tenant id.
func (r *Registry) TenantFor(address string) string {
	domain := address
	if i := strings.LastIndex(address, "@"); i >= 0 {
		domain = address[i+1:]
	}
	domain = strings.ToLower(domain)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.byDomain[domain]; ok {
		return t
	}
	return domain
}

// Quota enforces per-tenant daily caps. Safe for concurrent use.
type Quota struct {
	maxMsgs  int
	maxBytes int64
	now      func() time.Time

	mu    sync.Mutex
	usage map[string]*dayUsage
}

type dayUsage struct {
	day   int64
	msgs  int
	bytes int64
}

// NewQuota builds a quota with per-day message and byte caps (≤0 = unlimited).
func NewQuota(maxMsgs int, maxBytes int64, now func() time.Time) *Quota {
	if now == nil {
		now = time.Now
	}
	return &Quota{maxMsgs: maxMsgs, maxBytes: maxBytes, now: now, usage: map[string]*dayUsage{}}
}

// Allow checks whether a tenant can send another message of the given size and,
// if so, records it. Returns (false, reason) when over quota.
func (q *Quota) Allow(tenantID string, bytes int64) (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	day := q.now().UTC().Unix() / 86400
	u := q.usage[tenantID]
	if u == nil || u.day != day {
		u = &dayUsage{day: day}
		q.usage[tenantID] = u
	}
	if q.maxMsgs > 0 && u.msgs >= q.maxMsgs {
		return false, "daily message quota exceeded"
	}
	if q.maxBytes > 0 && u.bytes+bytes > q.maxBytes {
		return false, "daily storage quota exceeded"
	}
	u.msgs++
	u.bytes += bytes
	return true, ""
}
