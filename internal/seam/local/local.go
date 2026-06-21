// Package local provides the standalone, zero-dependency default implementations
// of the seam interfaces: a file-backed account store (bcrypt), permissive
// entitlements, and a no-op usage sink. Together they make vulos-mail a complete
// mail server with no external services — the OSS self-hosted path.
package local

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/vul-os/vulos-mail/internal/seam"
)

// Identity is a persistent, file-backed account store implementing seam.Identity.
// Credentials are bcrypt-hashed (over a sha256 prehash, so passwords longer than
// bcrypt's 72-byte limit aren't truncated) and persisted atomically to a JSON
// file under the data dir.
type Identity struct {
	path string

	mu    sync.Mutex
	users map[string][]byte // lower-cased address -> bcrypt(prehash(password))
}

// NewIdentity opens (or creates) the account store under dir/accounts.json.
func NewIdentity(dir string) (*Identity, error) {
	id := &Identity{path: filepath.Join(dir, "accounts.json"), users: map[string][]byte{}}
	if err := id.load(); err != nil {
		return nil, err
	}
	return id, nil
}

func (id *Identity) load() error {
	b, err := os.ReadFile(id.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for addr, b64 := range m {
		h, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return err
		}
		id.users[strings.ToLower(addr)] = h
	}
	return nil
}

// saveLocked atomically writes the store. Caller holds id.mu.
func (id *Identity) saveLocked() error {
	m := make(map[string]string, len(id.users))
	for addr, h := range id.users {
		m[addr] = base64.StdEncoding.EncodeToString(h)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := id.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, id.path)
}

func prehash(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

// Provision creates a new account, failing if it already exists.
func (id *Identity) Provision(_ context.Context, address, password string) error {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" || password == "" {
		return errors.New("local: address and password required")
	}
	h, err := bcrypt.GenerateFromPassword(prehash(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	if _, ok := id.users[address]; ok {
		return errors.New("local: account already exists")
	}
	id.users[address] = h
	return id.saveLocked()
}

// Upsert creates or replaces an account's password (used to seed an account from
// configuration, where the config is authoritative across restarts).
func (id *Identity) Upsert(address, password string) error {
	address = strings.ToLower(strings.TrimSpace(address))
	h, err := bcrypt.GenerateFromPassword(prehash(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	id.users[address] = h
	return id.saveLocked()
}

// Authenticate verifies credentials and returns the canonical address.
func (id *Identity) Authenticate(_ context.Context, username, password string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	id.mu.Lock()
	h, ok := id.users[username]
	id.mu.Unlock()
	if !ok {
		return "", errors.New("local: invalid credentials")
	}
	if bcrypt.CompareHashAndPassword(h, prehash(password)) != nil {
		return "", errors.New("local: invalid credentials")
	}
	return username, nil
}

// Exists reports whether an account is provisioned.
func (id *Identity) Exists(account string) bool {
	account = strings.ToLower(strings.TrimSpace(account))
	id.mu.Lock()
	defer id.mu.Unlock()
	_, ok := id.users[account]
	return ok
}

// Count returns the number of provisioned accounts.
func (id *Identity) Count() int {
	id.mu.Lock()
	defer id.mu.Unlock()
	return len(id.users)
}

// Entitlements is a fixed-plan entitlement source (the standalone default).
type Entitlements struct{ Plan seam.Plan }

// For returns the fixed plan for every account.
func (e Entitlements) For(context.Context, string) (seam.Plan, error) { return e.Plan, nil }

// Usage is a no-op metering sink (the standalone default).
type Usage struct{}

// Report discards the event.
func (Usage) Report(context.Context, seam.Event) {}
