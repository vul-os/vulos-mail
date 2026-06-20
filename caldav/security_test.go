package caldav

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// secAuth maps alice/secret -> account "alice".
func secAuth(user, pass string) (string, bool) {
	if user == "alice" && pass == "secret" {
		return "alice", true
	}
	return "", false
}

// TestCalDAVCrossAccountForbidden verifies an authenticated principal may only
// touch its OWN calendar collection: targeting another account's URL path is
// 403, even though the credentials are valid.
func TestCalDAVCrossAccountForbidden(t *testing.T) {
	b := New(secAuth, NewMemStore())
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()

	for _, m := range []string{"PROPFIND", "GET", "PUT", "DELETE", "REPORT"} {
		req, _ := http.NewRequest(m, srv.URL+"/dav/calendars/bob/evt.ics", strings.NewReader(""))
		req.SetBasicAuth("alice", "secret") // valid creds, wrong account
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s /dav/calendars/bob/...: want 403, got %d (cross-account access not blocked)", m, resp.StatusCode)
		}
	}
}

// TestCalDAVFSStorePathTraversal verifies the FSStore neutralizes "../",
// absolute paths and separators in both the account and href so a crafted DAV
// request can never read or write outside the store root.
func TestCalDAVFSStorePathTraversal(t *testing.T) {
	root := t.TempDir()
	// Plant a secret OUTSIDE the store root.
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
		{"../..", "../../secret.txt"},
		{"alice", "/etc/passwd"},
		{"alice", "..\\..\\secret.txt"},
	}

	for _, c := range evil {
		// Get must not read the planted secret.
		if data, ok := s.Get(c.account, c.href); ok && strings.Contains(string(data), "TOPSECRET") {
			t.Errorf("TRAVERSAL READ: Get(%q,%q) returned the out-of-root secret", c.account, c.href)
		}
		// Put must not write outside the store root.
		s.Put(c.account, c.href, []byte("pwned"))
	}

	// Every file the store created must live under storeRoot.
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if string(mustRead(t, p)) == "pwned" && !strings.HasPrefix(p, storeRoot) {
			t.Errorf("TRAVERSAL WRITE: store wrote outside its root at %q", p)
		}
		return nil
	})
	// The planted secret must be untouched.
	if string(mustRead(t, secret)) != "TOPSECRET" {
		t.Error("TRAVERSAL WRITE: the out-of-root secret was overwritten")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return b
}
