// Package byo implements the BYO ("bring your own") age-encrypted transient
// mail queue.
//
// Threat model:
//   - Inbound mail is encrypted with the recipient's X25519 public key (age
//     format) before it is handed to a relay. The relay holds only ciphertext;
//     it never possesses the private key and therefore cannot decrypt the
//     stored blobs.
//   - The recipient's private key (age identity) never leaves their instance;
//     decryption happens exclusively downstream where the identity lives.
//   - Queue metadata is deliberately minimal: account id, message id,
//     received-at, and attempts only. Subject, From, and To are NOT stored.
//
// This package provides the crypto helpers (key generation, Encrypt, Decrypt)
// and a concurrency-safe in-memory queue (Store / MemStore) for transient
// encrypted storage.
package byo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"filippo.io/age"
)

// ---------------------------------------------------------------------------
// Crypto helpers
// ---------------------------------------------------------------------------

// GenerateKeypair generates a fresh age X25519 keypair.
//
// publicKey is the recipient string ("age1..."), safe to share with a relay so
// it can encrypt mail to this account. privateKey is the identity string
// ("AGE-SECRET-KEY-...") and must never leave the recipient's instance.
func GenerateKeypair() (publicKey, privateKey string, err error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("byo: generate keypair: %w", err)
	}
	return id.Recipient().String(), id.String(), nil
}

// Encrypt age-encrypts plaintext to the recipient identified by recipientPubKey
// (an "age1..." public key string). The returned ciphertext is safe to store on
// an untrusted relay.
func Encrypt(recipientPubKey string, plaintext []byte) (ciphertext []byte, err error) {
	recip, err := age.ParseX25519Recipient(recipientPubKey)
	if err != nil {
		return nil, fmt.Errorf("byo: parse recipient: %w", err)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recip)
	if err != nil {
		return nil, fmt.Errorf("byo: create age writer: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("byo: encrypt write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("byo: finalise age writer: %w", err)
	}
	return buf.Bytes(), nil
}

// Decrypt decrypts an age ciphertext using privateKey (an "AGE-SECRET-KEY-..."
// identity string). Decrypting with a key other than the one the ciphertext was
// encrypted to returns an error.
func Decrypt(privateKey string, ciphertext []byte) (plaintext []byte, err error) {
	id, err := age.ParseX25519Identity(privateKey)
	if err != nil {
		return nil, fmt.Errorf("byo: parse identity: %w", err)
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), id)
	if err != nil {
		return nil, fmt.Errorf("byo: decrypt: %w", err)
	}
	plaintext, err = io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("byo: read plaintext: %w", err)
	}
	return plaintext, nil
}

// ---------------------------------------------------------------------------
// Queue
// ---------------------------------------------------------------------------

// Entry describes a queued encrypted blob and its minimal metadata.
//
// Deliberately excluded: subject, from, to. Only AccountID, MessageID,
// ReceivedAt and Attempts are retained. ReceivedAt is exported and settable so
// tests can simulate aged entries. Ciphertext is the age-encrypted blob.
type Entry struct {
	AccountID  string
	MessageID  string
	ReceivedAt time.Time
	Attempts   int
	Ciphertext []byte
}

// Store is the transient encrypted storage backend for the BYO queue.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Put stores ciphertext for the given account and message id, recording the
	// received-at time. Re-putting an existing (account, msgID) overwrites it.
	Put(account, msgID string, ciphertext []byte)
	// Fetch returns all entries for an account, ordered by ReceivedAt ascending.
	Fetch(account string) []Entry
	// Delete removes the entry for the given account and message id. It is a
	// no-op if the entry does not exist.
	Delete(account, msgID string)
}

// MemStore is a concurrency-safe in-memory implementation of Store. It is
// intended for transient storage only; nothing is persisted to disk.
type MemStore struct {
	mu      sync.Mutex
	entries map[string]map[string]Entry // account -> msgID -> Entry

	// now returns the current time; overridable for deterministic tests.
	now func() time.Time
}

// NewMemStore returns an empty, ready-to-use MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		entries: make(map[string]map[string]Entry),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

var _ Store = (*MemStore)(nil)

// SetNow overrides the clock used to stamp Entry.ReceivedAt on Put. It exists so
// tests can produce deterministic, settable received-at times; production code
// should leave the default (time.Now in UTC) in place.
func (m *MemStore) SetNow(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// Put stores ciphertext for the given account and message id. A defensive copy
// of ciphertext is taken so later mutations by the caller do not affect stored
// data. Re-putting an existing (account, msgID) overwrites the prior entry.
func (m *MemStore) Put(account, msgID string, ciphertext []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[account] == nil {
		m.entries[account] = make(map[string]Entry)
	}
	cp := make([]byte, len(ciphertext))
	copy(cp, ciphertext)
	m.entries[account][msgID] = Entry{
		AccountID:  account,
		MessageID:  msgID,
		ReceivedAt: m.now(),
		Attempts:   0,
		Ciphertext: cp,
	}
}

// Fetch returns all entries for an account ordered by ReceivedAt ascending
// (MessageID breaks ties for determinism). The returned slice is a snapshot;
// mutating it does not affect the store.
func (m *MemStore) Fetch(account string) []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	byMsg := m.entries[account]
	if len(byMsg) == 0 {
		return nil
	}
	out := make([]Entry, 0, len(byMsg))
	for _, e := range byMsg {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceivedAt.Equal(out[j].ReceivedAt) {
			return out[i].MessageID < out[j].MessageID
		}
		return out[i].ReceivedAt.Before(out[j].ReceivedAt)
	})
	return out
}

// Delete removes the entry for the given account and message id. It is a no-op
// if no such entry exists.
func (m *MemStore) Delete(account, msgID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if byMsg := m.entries[account]; byMsg != nil {
		delete(byMsg, msgID)
		if len(byMsg) == 0 {
			delete(m.entries, account)
		}
	}
}

// ErrNotFound is returned by callers that look up a missing entry. It is
// provided for consumers building higher-level lookups on top of Store.
var ErrNotFound = errors.New("byo: not found")
