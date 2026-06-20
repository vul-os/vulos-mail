// Package snooze implements a time-based scheduler for deferred mail actions:
// un-snoozing a message at a future time and sending a message at a future
// time (scheduled send).
//
// It is a port of vulos-mail's snooze feature, generalised into a due-time
// queue keyed by an absolute DueAt instant. Each deferred action is an Item;
// a Store holds pending Items and surfaces those whose DueAt has passed; a
// Scheduler wraps a Store with an injectable clock so behaviour is fully
// deterministic in tests.
//
// The package is stdlib-only and concurrency-safe.
package snooze

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Kind enumerates the supported deferred-action kinds carried by an Item.
const (
	// KindSnooze marks a message to be un-snoozed (restored to the inbox)
	// when its DueAt is reached.
	KindSnooze = "snooze"
	// KindSend marks a message to be sent when its DueAt is reached
	// (scheduled send).
	KindSend = "send"
)

// Item is a single deferred action scheduled to fire at DueAt.
type Item struct {
	// ID uniquely identifies the item within a Store.
	ID string
	// Account is the owning account identifier.
	Account string
	// Kind is the action kind, e.g. KindSnooze or KindSend.
	Kind string
	// DueAt is the absolute time at which the action becomes due.
	DueAt time.Time
	// Payload is opaque action-specific data (e.g. an encoded message).
	Payload []byte
}

// Store is the persistence seam for pending Items. Implementations must be
// safe for concurrent use.
type Store interface {
	// Add stores a pending item. An existing item with the same ID is
	// replaced (upsert).
	Add(item Item)
	// Due removes and returns all items whose DueAt is at or before now.
	Due(now time.Time) []Item
	// Cancel removes the pending item with the given ID, if present.
	Cancel(id string)
	// Pending reports the number of items currently stored.
	Pending() int
}

// MemStore is a concurrency-safe, in-memory Store.
type MemStore struct {
	mu    sync.Mutex
	items map[string]Item
}

// NewMemStore returns an empty in-memory Store.
func NewMemStore() *MemStore {
	return &MemStore{items: make(map[string]Item)}
}

// Add stores (or replaces) a pending item keyed by its ID.
func (s *MemStore) Add(item Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[item.ID] = item
}

// Due removes and returns every item whose DueAt is at or before now.
func (s *MemStore) Due(now time.Time) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Item
	for id, it := range s.items {
		if !it.DueAt.After(now) {
			due = append(due, it)
			delete(s.items, id)
		}
	}
	return due
}

// Cancel removes the pending item with the given ID. It is a no-op if no
// such item exists.
func (s *MemStore) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

// Pending reports the number of items currently stored.
func (s *MemStore) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// Scheduler wraps a Store with an injectable clock and exposes the operations
// used by callers to defer, cancel, and drain due actions.
type Scheduler struct {
	store Store
	now   func() time.Time
}

// New returns a Scheduler backed by store. now supplies the current time; if
// nil, time.Now is used.
func New(store Store, now func() time.Time) *Scheduler {
	if now == nil {
		now = time.Now
	}
	return &Scheduler{store: store, now: now}
}

// Schedule adds item to the underlying store.
func (sc *Scheduler) Schedule(item Item) {
	sc.store.Add(item)
}

// Cancel removes a pending item by ID.
func (sc *Scheduler) Cancel(id string) {
	sc.store.Cancel(id)
}

// Pending reports the number of items currently scheduled.
func (sc *Scheduler) Pending() int {
	return sc.store.Pending()
}

// Due removes and returns all items whose DueAt is at or before the current
// clock time, ordered by ascending DueAt (ties broken by ID for stability).
func (sc *Scheduler) Due() []Item {
	due := sc.store.Due(sc.now())
	sort.Slice(due, func(i, j int) bool {
		if due[i].DueAt.Equal(due[j].DueAt) {
			return due[i].ID < due[j].ID
		}
		return due[i].DueAt.Before(due[j].DueAt)
	})
	return due
}

// Run drains due items to handler on each tick of interval until ctx is
// cancelled. It processes any already-due items once immediately on start
// (handling restart survival), then ticks. Run blocks until ctx.Done.
func (sc *Scheduler) Run(ctx context.Context, interval time.Duration, handler func(Item)) {
	drain := func() {
		for _, it := range sc.Due() {
			handler(it)
		}
	}
	drain() // process overdue items on startup
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drain()
		}
	}
}
