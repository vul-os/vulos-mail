package carddav

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
// address-book resources so contacts survive restarts and are shared between the
// CardDAV server and the webmail contacts API.
type FSStore struct {
	root string
	mu   sync.Mutex
}

// NewFSStore creates a persistent store rooted at root.
func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &FSStore{root: root}, nil
}

func safe(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "..", "_", "@", "_at_").Replace(s)
}

func (s *FSStore) dir(account string) string {
	return filepath.Join(s.root, safe(strings.ToLower(account)))
}

func fsEtag(data []byte) string {
	h := sha256.Sum256(data)
	return `"` + hex.EncodeToString(h[:]) + `"`
}

func (s *FSStore) Put(account, href string, data []byte) (Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.dir(account)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return Resource{}, err
	}
	if err := os.WriteFile(filepath.Join(d, safe(href)), data, 0o600); err != nil {
		return Resource{}, err
	}
	return Resource{Href: href, Data: data, ETag: fsEtag(data)}, nil
}

func (s *FSStore) Get(account, href string) (Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.dir(account), safe(href)))
	if os.IsNotExist(err) {
		return Resource{}, ErrNotFound
	}
	if err != nil {
		return Resource{}, err
	}
	return Resource{Href: href, Data: data, ETag: fsEtag(data)}, nil
}

func (s *FSStore) Delete(account, href string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(filepath.Join(s.dir(account), safe(href)))
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}

func (s *FSStore) List(account string) ([]Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir(account))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	return out, nil
}
