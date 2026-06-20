package caldav

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FSStore is a filesystem-backed Store: <root>/<account>/<href>. It persists
// calendar resources so events survive restarts and are shared between the
// CalDAV server and the webmail calendar API.
type FSStore struct {
	root string
	mu   sync.Mutex
}

// NewFSStore creates a persistent calendar store rooted at root.
func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &FSStore{root: root}, nil
}

func fsSafe(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "..", "_", "@", "_at_").Replace(strings.ToLower(s))
}

func (s *FSStore) dir(account string) string { return filepath.Join(s.root, fsSafe(account)) }

func fsEtag(data []byte) string { h := sha256.Sum256(data); return `"` + hex.EncodeToString(h[:]) + `"` }

func (s *FSStore) Put(account, href string, ics []byte) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.dir(account)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return ""
	}
	if err := os.WriteFile(filepath.Join(d, fsSafe(href)), ics, 0o600); err != nil {
		return ""
	}
	return fsEtag(ics)
}

func (s *FSStore) Get(account, href string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.dir(account), fsSafe(href)))
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *FSStore) Delete(account, href string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(filepath.Join(s.dir(account), fsSafe(href))) == nil
}

func (s *FSStore) List(account string) []Resource {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir(account))
	if err != nil {
		return nil
	}
	var out []Resource
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir(account), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, Resource{Href: e.Name(), Data: data, ETag: fsEtag(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Href < out[j].Href })
	return out
}
