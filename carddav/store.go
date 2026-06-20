package carddav

import (
	"errors"
	"sort"
	"sync"
)

// ErrNotFound is returned by a Store when a requested resource does not exist.
var ErrNotFound = errors.New("carddav: resource not found")

// Resource is a single stored vCard, identified within an account's address
// book by its href (the resource filename, e.g. "alice.vcf").
type Resource struct {
	// Href is the resource name within the address book collection. It is the
	// last path segment of the resource URL and conventionally ends in ".vcf".
	Href string
	// Data is the raw vCard body as stored on PUT.
	Data []byte
	// ETag is the strong entity tag for Data (a quoted content hash).
	ETag string
}

// Store is the persistence boundary for CardDAV address books. Each account has
// its own flat namespace of resources keyed by href. Implementations must be
// safe for concurrent use.
type Store interface {
	// Put stores (creating or replacing) the resource at href for account,
	// returning the stored resource including its computed ETag.
	Put(account, href string, data []byte) (Resource, error)
	// Get returns the resource at href for account, or ErrNotFound.
	Get(account, href string) (Resource, error)
	// Delete removes the resource at href for account, returning ErrNotFound if
	// it does not exist.
	Delete(account, href string) error
	// List returns all resources in account's address book, sorted by href.
	List(account string) ([]Resource, error)
}

// MemStore is an in-memory, concurrency-safe Store suitable for tests and
// single-process deployments. The zero value is not usable; call NewMemStore.
type MemStore struct {
	mu    sync.RWMutex
	books map[string]map[string]Resource // account -> href -> resource
}

// NewMemStore returns an empty in-memory Store.
func NewMemStore() *MemStore {
	return &MemStore{books: make(map[string]map[string]Resource)}
}

// Put implements Store.
func (m *MemStore) Put(account, href string, data []byte) (Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	book := m.books[account]
	if book == nil {
		book = make(map[string]Resource)
		m.books[account] = book
	}
	// Copy the body so callers cannot mutate stored bytes.
	buf := make([]byte, len(data))
	copy(buf, data)
	r := Resource{Href: href, Data: buf, ETag: etag(buf)}
	book[href] = r
	return r, nil
}

// Get implements Store.
func (m *MemStore) Get(account, href string) (Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.books[account][href]
	if !ok {
		return Resource{}, ErrNotFound
	}
	return r, nil
}

// Delete implements Store.
func (m *MemStore) Delete(account, href string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	book := m.books[account]
	if _, ok := book[href]; !ok {
		return ErrNotFound
	}
	delete(book, href)
	return nil
}

// List implements Store.
func (m *MemStore) List(account string) ([]Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	book := m.books[account]
	out := make([]Resource, 0, len(book))
	for _, r := range book {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Href < out[j].Href })
	return out, nil
}
