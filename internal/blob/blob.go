// Package blob is the content-addressed, compressed, deduplicated object store
// for immutable message bodies. The FS impl is for dev/tests; an S3 impl is a
// later drop-in behind the Store interface. Compression is gzip (stdlib) today;
// zstd (klauspost/compress) is a later swap — the on-disk format is internal and
// integrity-checked on read, so it can change freely.
package blob

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vul-os/vmail/internal/model"
)

// ErrNotFound is returned by Get when a ref is absent.
var ErrNotFound = errors.New("blob: not found")

// Store is the object store contract. Put is idempotent (content-addressed), so
// identical bodies dedup automatically across the whole system.
type Store interface {
	Put(ctx context.Context, data []byte) (model.BlobRef, error)
	Get(ctx context.Context, ref model.BlobRef) ([]byte, error)
	Has(ctx context.Context, ref model.BlobRef) (bool, error)
}

// Ref computes the content-addressed ref for data.
func Ref(data []byte) model.BlobRef {
	sum := sha256.Sum256(data)
	return model.BlobRef("sha256:" + hex.EncodeToString(sum[:]))
}

// FS is a filesystem-backed store. Layout: <root>/<hash[:2]>/<hash>, gzip body.
type FS struct {
	root string
}

// NewFS creates a store rooted at root.
func NewFS(root string) (*FS, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &FS{root: root}, nil
}

func (s *FS) path(ref model.BlobRef) (string, error) {
	h := strings.TrimPrefix(string(ref), "sha256:")
	if len(h) < 2 {
		return "", errors.New("blob: malformed ref")
	}
	return filepath.Join(s.root, h[:2], h), nil
}

func (s *FS) Put(_ context.Context, data []byte) (model.BlobRef, error) {
	ref := Ref(data)
	p, err := s.path(ref)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return ref, nil // already present — dedup
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	// Atomic publish via temp + rename.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return ref, nil
}

func (s *FS) Get(_ context.Context, ref model.BlobRef) ([]byte, error) {
	p, err := s.path(ref)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if Ref(out) != ref {
		return nil, errors.New("blob: integrity mismatch")
	}
	return out, nil
}

func (s *FS) Has(_ context.Context, ref model.BlobRef) (bool, error) {
	p, err := s.path(ref)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
