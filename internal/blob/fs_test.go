package blob_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/model"
)

func newStore(t *testing.T) *blob.FS {
	t.Helper()
	s, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return s
}

func TestFSPutIsContentAddressedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	data := []byte("hello, content-addressed world")

	ref1, err := s.Put(ctx, data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Ref must be the content hash.
	if want := blob.Ref(data); ref1 != want {
		t.Errorf("Put ref = %q, want content-address %q", ref1, want)
	}

	// Putting identical bytes again yields the same ref (dedup).
	ref2, err := s.Put(ctx, data)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if ref1 != ref2 {
		t.Errorf("idempotent Put gave different refs: %q vs %q", ref1, ref2)
	}

	// And it must be stored exactly once.
	infos, err := s.ListBlobs(ctx)
	if err != nil {
		t.Fatalf("ListBlobs: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("after duplicate Put, ListBlobs len = %d, want 1 (dedup)", len(infos))
	}

	// Different content produces a different ref.
	otherRef, err := s.Put(ctx, []byte("different bytes"))
	if err != nil {
		t.Fatalf("Put other: %v", err)
	}
	if otherRef == ref1 {
		t.Error("distinct content produced the same ref")
	}
}

func TestFSGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	cases := map[string][]byte{
		"empty":  {},
		"small":  []byte("the quick brown fox"),
		"binary": {0x00, 0x01, 0xff, 0xfe, 0x7f, 0x80},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			ref, err := s.Put(ctx, data)
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := s.Get(ctx, ref)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(data))
			}
		})
	}
}

// A ~1MB body exercises the zstd compress/decompress path on real volume.
func TestFSGetRoundTripLarge(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Mix of random (incompressible) and repetitive (compressible) data so we
	// exercise zstd both ways.
	large := make([]byte, 1<<20) // 1 MiB
	if _, err := rand.Read(large[:1<<19]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	for i := 1 << 19; i < len(large); i++ {
		large[i] = byte(i % 251)
	}

	ref, err := s.Put(ctx, large)
	if err != nil {
		t.Fatalf("Put large: %v", err)
	}
	got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get large: %v", err)
	}
	if !bytes.Equal(got, large) {
		t.Fatalf("large round-trip mismatch: got %d bytes, want %d", len(got), len(large))
	}
}

func TestFSGetAbsentReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	absent := blob.Ref([]byte("never stored"))
	_, err := s.Get(ctx, absent)
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("Get absent err = %v, want ErrNotFound", err)
	}
}

func TestFSHas(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	data := []byte("presence check")
	absent := blob.Ref([]byte("not here"))

	if has, err := s.Has(ctx, absent); err != nil || has {
		t.Fatalf("Has(absent) = (%v, %v), want (false, nil)", has, err)
	}

	ref, err := s.Put(ctx, data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if has, err := s.Has(ctx, ref); err != nil || !has {
		t.Fatalf("Has(present) = (%v, %v), want (true, nil)", has, err)
	}
}

func TestFSListBlobs(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if infos, err := s.ListBlobs(ctx); err != nil || len(infos) != 0 {
		t.Fatalf("ListBlobs(empty) = (%v, %v), want (empty, nil)", infos, err)
	}

	want := map[model.BlobRef]bool{}
	for _, payload := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		ref, err := s.Put(ctx, payload)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		want[ref] = true
	}

	infos, err := s.ListBlobs(ctx)
	if err != nil {
		t.Fatalf("ListBlobs: %v", err)
	}
	if len(infos) != len(want) {
		t.Errorf("ListBlobs len = %d, want %d", len(infos), len(want))
	}
	for _, info := range infos {
		if !want[info.Ref] {
			t.Errorf("ListBlobs returned unexpected ref %q", info.Ref)
		}
		if info.ModTime.IsZero() {
			t.Errorf("ListBlobs ref %q has zero mod time", info.Ref)
		}
	}
}

func TestFSDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	data := []byte("to be deleted")
	ref, err := s.Put(ctx, data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := s.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if has, err := s.Has(ctx, ref); err != nil || has {
		t.Errorf("Has after Delete = (%v, %v), want (false, nil)", has, err)
	}
	if _, err := s.Get(ctx, ref); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Get after Delete err = %v, want ErrNotFound", err)
	}

	// Delete of an absent ref is a no-op (idempotent), not an error.
	if err := s.Delete(ctx, ref); err != nil {
		t.Errorf("Delete of absent ref = %v, want nil", err)
	}
}
