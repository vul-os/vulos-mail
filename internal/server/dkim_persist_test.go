package server_test

import (
	"path/filepath"
	"testing"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/server"
)

// DKIM keys must be stable across restarts — the published DNS TXT is derived
// from them, so a regenerated key would silently break DKIM for the domain.
func TestDKIMKeyPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	blobs, _ := blob.NewFS(filepath.Join(dir, "blobs"))

	m1 := server.NewManager(dir, blobs, nil)
	txt1, err := m1.EnsureDKIM("vmail.test", "vmail")
	if err != nil || txt1 == "" {
		t.Fatalf("EnsureDKIM #1: %q %v", txt1, err)
	}

	// Fresh manager, same data dir = simulated restart.
	m2 := server.NewManager(dir, blobs, nil)
	txt2, err := m2.EnsureDKIM("vmail.test", "vmail")
	if err != nil {
		t.Fatal(err)
	}
	if txt2 != txt1 {
		t.Fatalf("DKIM TXT changed across restart:\n%s\nvs\n%s", txt1, txt2)
	}
}
