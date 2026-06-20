// Package caldav implements a minimal RFC 4791 CalDAV server exposed as an
// http.Handler. It stores iCalendar resources per account and supports the
// core verbs a calendar client needs: PROPFIND on a calendar collection,
// PUT/GET/DELETE of individual .ics resources, and a calendar-query REPORT
// with a time-range filter.
//
// The distinguishing feature over the legacy vulos-mail calendar is RRULE
// expansion: a calendar-query time-range REPORT expands recurring VEVENTs
// (via github.com/teambition/rrule-go) so that individual recurrence
// instances falling inside the requested window are matched and returned.
//
// iCalendar parsing and encoding are delegated to
// github.com/emersion/go-ical. Authentication is HTTP Basic, delegated to a
// caller-supplied function. Storage is abstracted behind the Store interface;
// an in-memory implementation (MemStore) is provided.
package caldav

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
)

// Resource is a single stored iCalendar object within an account's calendar
// collection. Href is the resource path component (filename), typically
// ending in ".ics". Data holds the raw iCalendar bytes. ETag is a strong
// entity tag derived from Data.
type Resource struct {
	Href string
	Data []byte
	ETag string
}

// Store is a per-account calendar collection of iCalendar resources keyed by
// href (filename). Implementations must be safe for concurrent use.
type Store interface {
	// Put stores (or replaces) the resource at href for the account and
	// returns the resulting ETag.
	Put(account, href string, ics []byte) (etag string)
	// Get returns the raw iCalendar bytes for href, and whether it exists.
	Get(account, href string) (ics []byte, ok bool)
	// Delete removes the resource at href. It reports whether a resource was
	// removed.
	Delete(account, href string) (existed bool)
	// List returns all resources for the account, sorted by href.
	List(account string) []Resource
}

// MemStore is an in-memory, concurrency-safe Store implementation suitable for
// tests and small deployments.
type MemStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte // account -> href -> ics
}

// NewMemStore returns an empty MemStore ready for use.
func NewMemStore() *MemStore {
	return &MemStore{data: make(map[string]map[string][]byte)}
}

// ETag computes a strong entity tag (a quoted SHA-256 hex digest) for the
// given iCalendar bytes.
func ETag(ics []byte) string {
	sum := sha256.Sum256(ics)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// Put implements Store.
func (m *MemStore) Put(account, href string, ics []byte) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	coll := m.data[account]
	if coll == nil {
		coll = make(map[string][]byte)
		m.data[account] = coll
	}
	buf := make([]byte, len(ics))
	copy(buf, ics)
	coll[href] = buf
	return ETag(buf)
}

// Get implements Store.
func (m *MemStore) Get(account, href string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	coll := m.data[account]
	if coll == nil {
		return nil, false
	}
	ics, ok := coll[href]
	if !ok {
		return nil, false
	}
	buf := make([]byte, len(ics))
	copy(buf, ics)
	return buf, true
}

// Delete implements Store.
func (m *MemStore) Delete(account, href string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	coll := m.data[account]
	if coll == nil {
		return false
	}
	if _, ok := coll[href]; !ok {
		return false
	}
	delete(coll, href)
	return true
}

// List implements Store.
func (m *MemStore) List(account string) []Resource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	coll := m.data[account]
	out := make([]Resource, 0, len(coll))
	for href, ics := range coll {
		buf := make([]byte, len(ics))
		copy(buf, ics)
		out = append(out, Resource{Href: href, Data: buf, ETag: ETag(buf)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Href < out[j].Href })
	return out
}
