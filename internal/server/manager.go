// Package server assembles the pieces into a running multi-account mail system:
// it manages per-account runtimes (each its own durable log over a shared,
// deduplicated blob store) and exposes the callbacks the protocol adapters need
// — MX delivery, IMAP auth, submission auth. This is the wiring layer; cmd/vmail
// turns it into a process.
//
// Auth here is a placeholder in-memory credential map; OAuth2/TOTP/passkeys are a
// later wave. Account addressing is exact-match (one address = one account).
package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/services/mtaout"
)

// Manager owns all accounts and the shared infrastructure.
type Manager struct {
	dir   string
	blobs blob.Store
	gen   *ids.Gen
	sched *mtaout.Scheduler

	mu       sync.Mutex
	accounts map[string]*account.Runtime
	creds    map[string]string // address -> password (placeholder)
}

// NewManager creates a manager rooted at dir, using blobs for bodies and sched
// for outbound (sched may be nil if sending is disabled).
func NewManager(dir string, blobs blob.Store, sched *mtaout.Scheduler) *Manager {
	return &Manager{
		dir: dir, blobs: blobs, gen: ids.NewGen(), sched: sched,
		accounts: map[string]*account.Runtime{},
		creds:    map[string]string{},
	}
}

// AddAccount registers an address with a password (placeholder provisioning).
func (m *Manager) AddAccount(address, password string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[strings.ToLower(address)] = password
}

// account returns the runtime for address, opening (and caching) it on first use.
func (m *Manager) account(ctx context.Context, address string) (*account.Runtime, error) {
	address = strings.ToLower(address)
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.accounts[address]; ok {
		return rt, nil
	}
	if _, ok := m.creds[address]; !ok {
		return nil, errors.New("no such account")
	}
	acctDir := filepath.Join(m.dir, "accounts", safeName(address))
	if err := os.MkdirAll(acctDir, 0o700); err != nil {
		return nil, err
	}
	log, err := eventlog.OpenFile(filepath.Join(acctDir, "log.jsonl"), nil)
	if err != nil {
		return nil, err
	}
	rt, err := account.Open(ctx, log, m.blobs, m.gen, nil)
	if err != nil {
		return nil, err
	}
	m.accounts[address] = rt
	return rt, nil
}

// Deliver is the MX delivery callback: route an accepted message to a recipient's
// account inbox. Unknown recipients are rejected (the MX returns an error).
func (m *Manager) Deliver(ctx context.Context, rcpt string, raw []byte) error {
	rt, err := m.account(ctx, rcpt)
	if err != nil {
		return err
	}
	_, err = rt.Ingest(ctx, raw, []model.LabelID{model.LabelInbox}, nil)
	return err
}

// AuthIMAP validates IMAP credentials and returns the account runtime.
func (m *Manager) AuthIMAP(username, password string) (*account.Runtime, error) {
	if !m.checkCred(username, password) {
		return nil, errors.New("invalid credentials")
	}
	return m.account(context.Background(), username)
}

// AuthSubmit validates submission credentials and returns the runtime + tenant.
func (m *Manager) AuthSubmit(username, password string) (*account.Runtime, string, error) {
	if !m.checkCred(username, password) {
		return nil, "", errors.New("invalid credentials")
	}
	rt, err := m.account(context.Background(), username)
	if err != nil {
		return nil, "", err
	}
	return rt, tenantOf(username), nil
}

// Enqueue hands an outbound message to the scheduler (used by the submission
// backend's Enqueue hook).
func (m *Manager) Enqueue(msg mtaout.OutMessage) {
	if m.sched != nil {
		m.sched.Enqueue(msg)
	}
}

func (m *Manager) checkCred(username, password string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	want, ok := m.creds[strings.ToLower(username)]
	return ok && want == password
}

func tenantOf(address string) string {
	if i := strings.LastIndex(address, "@"); i >= 0 {
		return address[i+1:]
	}
	return address
}

func safeName(address string) string {
	return strings.NewReplacer("@", "_at_", "/", "_", "\\", "_", "..", "_").Replace(address)
}
