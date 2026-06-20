package carddav

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func secAuth(user, pass string) (string, bool) {
	if user == "alice" && pass == "secret" {
		return "alice", true
	}
	return "", false
}

// TestCardDAVCrossAccountForbidden verifies a valid principal cannot operate on
// another account's address book via the URL path (403).
func TestCardDAVCrossAccountForbidden(t *testing.T) {
	store, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Auth: Auth(secAuth), Store: store}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	for _, m := range []string{"PROPFIND", "GET", "PUT", "DELETE", "REPORT"} {
		req, _ := http.NewRequest(m, srv.URL+"/dav/addressbooks/bob/card.vcf", strings.NewReader(""))
		req.SetBasicAuth("alice", "secret") // valid creds, wrong account
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s /dav/addressbooks/bob/...: want 403, got %d", m, resp.StatusCode)
		}
	}
}

// TestCardDAVFSStorePathTraversal verifies the FSStore neutralizes traversal in
// account and href so no read/write escapes the store root.
func TestCardDAVFSStorePathTraversal(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	storeRoot := filepath.Join(root, "store")
	s, err := NewFSStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}

	evil := []struct{ account, href string }{
		{"alice", "../../secret.txt"},
		{"alice", "../secret.txt"},
		{"..", "secret.txt"},
		{"alice", "/etc/passwd"},
		{"alice", "..\\..\\secret.txt"},
	}
	for _, c := range evil {
		if res, err := s.Get(c.account, c.href); err == nil && strings.Contains(string(res.Data), "TOPSECRET") {
			t.Errorf("TRAVERSAL READ: Get(%q,%q) returned the out-of-root secret", c.account, c.href)
		}
		_, _ = s.Put(c.account, c.href, []byte("pwned"))
	}

	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil && string(b) == "pwned" && !strings.HasPrefix(p, storeRoot) {
			t.Errorf("TRAVERSAL WRITE: store wrote outside its root at %q", p)
		}
		return nil
	})
	if b, _ := os.ReadFile(secret); string(b) != "TOPSECRET" {
		t.Error("TRAVERSAL WRITE: the out-of-root secret was overwritten")
	}
}
