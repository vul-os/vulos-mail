package blob

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/vul-os/vmail/internal/model"
)

func TestObjectKey(t *testing.T) {
	ref := Ref([]byte("hello"))
	key, err := objectKey(ref)
	if err != nil {
		t.Fatal(err)
	}
	h, err := hashOf(ref)
	if err != nil {
		t.Fatal(err)
	}
	want := h[:2] + "/" + h
	if key != want {
		t.Fatalf("objectKey = %q, want %q", key, want)
	}
	if key[2] != '/' {
		t.Fatalf("key not sharded: %q", key)
	}
}

func TestObjectKeyMalformed(t *testing.T) {
	if _, err := objectKey(model.BlobRef("sha256:")); err == nil {
		t.Fatal("expected error for malformed ref")
	}
}

// TestS3RoundTrip exercises a real S3-compatible endpoint. It is skipped unless
// VMAIL_TEST_S3_ENDPOINT is set, so CI never needs network. Example:
//
//	VMAIL_TEST_S3_ENDPOINT=localhost:9000 \
//	VMAIL_TEST_S3_ACCESS=minioadmin VMAIL_TEST_S3_SECRET=minioadmin \
//	VMAIL_TEST_S3_BUCKET=vmail-test go test ./internal/blob/...
func TestS3RoundTrip(t *testing.T) {
	endpoint := os.Getenv("VMAIL_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set VMAIL_TEST_S3_ENDPOINT to run the live S3 round-trip test")
	}
	bucket := os.Getenv("VMAIL_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "vmail-test"
	}
	ctx := context.Background()
	s, err := NewS3(ctx, endpoint,
		os.Getenv("VMAIL_TEST_S3_ACCESS"),
		os.Getenv("VMAIL_TEST_S3_SECRET"),
		bucket,
		os.Getenv("VMAIL_TEST_S3_SSL") == "1",
	)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("Subject: hi\r\n\r\nhello s3\r\n")
	ref, err := s.Put(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if ref != Ref(data) {
		t.Fatalf("ref = %q, want %q", ref, Ref(data))
	}

	got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("roundtrip mismatch")
	}

	// Dedup: a second Put of identical content is a no-op returning the same ref.
	ref2, err := s.Put(ctx, data)
	if err != nil || ref2 != ref {
		t.Fatalf("dedup put: ref=%q err=%v", ref2, err)
	}

	has, err := s.Has(ctx, ref)
	if err != nil || !has {
		t.Fatalf("Has = %v, %v", has, err)
	}

	absent := Ref([]byte("definitely-absent-from-s3"))
	if has, err := s.Has(ctx, absent); err != nil || has {
		t.Fatalf("Has(absent) = %v, %v", has, err)
	}
	if _, err := s.Get(ctx, absent); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(absent) = %v, want ErrNotFound", err)
	}
}
