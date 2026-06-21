package blob

import (
	"context"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/model"
)

// A crafted ref must never resolve to a path/key outside the store root. hashOf
// now requires a 64-char lowercase hex digest, so traversal refs are rejected
// before any filesystem path is built.
func TestRefTraversalRejected(t *testing.T) {
	bad := []model.BlobRef{
		"sha256:../../etc/passwd",
		"sha256:..",
		"sha256:/etc/passwd",
		model.BlobRef("sha256:" + strings.Repeat("a", 63)), // too short
		model.BlobRef("sha256:" + strings.Repeat("a", 65)), // too long
		model.BlobRef("sha256:" + strings.Repeat("A", 64)), // uppercase not allowed
		model.BlobRef("sha256:" + strings.Repeat("g", 64)), // non-hex
		model.BlobRef("md5:" + strings.Repeat("a", 64)),    // wrong algo prefix
		"../leak",
		"",
	}
	for _, ref := range bad {
		if _, err := hashOf(ref); err == nil {
			t.Errorf("hashOf(%q) accepted a malformed/traversal ref", ref)
		}
	}

	// FS operations on a crafted ref error out (no escape, no panic).
	ctx := context.Background()
	s, _ := NewFS(t.TempDir())
	evil := model.BlobRef("sha256:../../../../../../tmp/vulos-leak")
	if _, err := s.Get(ctx, evil); err == nil {
		t.Error("Get with traversal ref should error")
	}
	if err := s.Delete(ctx, evil); err == nil {
		t.Error("Delete with traversal ref should error")
	}
	if _, err := s.Has(ctx, evil); err == nil {
		t.Error("Has with traversal ref should error")
	}

	// A real ref still round-trips.
	ref, err := s.Put(ctx, []byte("legit"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, ref); err != nil {
		t.Fatalf("valid ref should round-trip: %v", err)
	}
}
