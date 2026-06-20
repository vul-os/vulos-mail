package server_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/server"
)

// GC deletes blobs no live message references, and keeps referenced ones.
func TestGCBlobsSweepsOrphans(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	blobs, _ := blob.NewFS(filepath.Join(dir, "blobs"))
	mgr := server.NewManager(dir, blobs, nil)
	_ = mgr.AddAccount("alice@vmail.test", "pw")

	// A live message (its blob is referenced).
	if err := mgr.Deliver(ctx, "alice@vmail.test", []byte("From: x@y\r\nTo: alice@vmail.test\r\nSubject: keep\r\n\r\nlive body\r\n")); err != nil {
		t.Fatal(err)
	}
	liveRef := blob.Ref([]byte("From: x@y\r\nTo: alice@vmail.test\r\nSubject: keep\r\n\r\nlive body\r\n"))

	// An orphan blob never referenced by any message.
	orphan, err := blobs.Put(ctx, []byte("orphan bytes nobody references"))
	if err != nil {
		t.Fatal(err)
	}

	n, err := mgr.GCBlobs(ctx, 0) // grace 0 = sweep everything unreferenced
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("GC removed %d, want 1 (the orphan)", n)
	}
	if ok, _ := blobs.Has(ctx, orphan); ok {
		t.Error("orphan blob should have been deleted")
	}
	if ok, _ := blobs.Has(ctx, liveRef); !ok {
		t.Error("live blob must NOT be deleted")
	}
	// The live message is still readable.
	rt, _ := mgr.AuthIMAP("alice@vmail.test", "pw")
	if got := rt.MessagesWithLabel(model.LabelInbox); len(got) != 1 {
		t.Fatalf("inbox should still have its message, got %d", len(got))
	}
}
