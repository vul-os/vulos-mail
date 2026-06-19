package blob_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/vul-os/vmail/internal/blob"
)

func TestPutGetDedupIntegrity(t *testing.T) {
	ctx := context.Background()
	s, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("Subject: hi\r\n\r\nhello world\r\n")
	ref, err := s.Put(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if ref != blob.Ref(data) {
		t.Fatalf("ref = %q, want %q", ref, blob.Ref(data))
	}

	got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("roundtrip mismatch")
	}

	// Dedup: putting identical content returns the same ref without error.
	ref2, err := s.Put(ctx, data)
	if err != nil || ref2 != ref {
		t.Fatalf("dedup put: ref=%q err=%v", ref2, err)
	}

	has, err := s.Has(ctx, ref)
	if err != nil || !has {
		t.Fatalf("Has = %v, %v", has, err)
	}

	if _, err := s.Get(ctx, blob.Ref([]byte("absent"))); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRefStable(t *testing.T) {
	a := blob.Ref([]byte("x"))
	b := blob.Ref([]byte("x"))
	if a != b {
		t.Fatal("Ref must be deterministic")
	}
	if a == blob.Ref([]byte("y")) {
		t.Fatal("distinct content must have distinct refs")
	}
}
