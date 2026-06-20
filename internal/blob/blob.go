// Package blob is the content-addressed, compressed, deduplicated object store
// for immutable message bodies. The FS impl is for dev/tests; the S3 impl
// (see s3.go) is a drop-in behind the Store interface. Compression is zstd
// (klauspost/compress); the on-disk/object format is internal and
// integrity-checked on read, so it can change freely.
package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/vul-os/vulos-mail/internal/model"
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

// BlobInfo is a stored blob with its modification time (used by GC for a grace
// window so a just-Put blob isn't swept before its referencing event commits).
type BlobInfo struct {
	Ref     model.BlobRef
	ModTime time.Time
}

// GCStore is a Store that supports garbage collection (enumerate + delete).
type GCStore interface {
	Store
	ListBlobs(ctx context.Context) ([]BlobInfo, error)
	Delete(ctx context.Context, ref model.BlobRef) error
}

// Ref computes the content-addressed ref for data.
func Ref(data []byte) model.BlobRef {
	sum := sha256.Sum256(data)
	return model.BlobRef("sha256:" + hex.EncodeToString(sum[:]))
}

// Shared zstd encoder/decoder. klauspost's Encoder and Decoder are safe for
// concurrent use, so a single package-level instance serves every Put/Get.
// EncodeAll/DecodeAll are stateless across calls, so this is correct and avoids
// per-op allocation of the underlying compression state.
var (
	zstdEnc *zstd.Encoder
	zstdDec *zstd.Decoder
)

func init() {
	var err error
	zstdEnc, err = zstd.NewWriter(nil)
	if err != nil {
		panic("blob: zstd encoder init: " + err.Error())
	}
	zstdDec, err = zstd.NewReader(nil)
	if err != nil {
		panic("blob: zstd decoder init: " + err.Error())
	}
}

// compress zstd-compresses plaintext into a self-contained frame.
func compress(plain []byte) []byte {
	return zstdEnc.EncodeAll(plain, nil)
}

// decompress reverses compress.
func decompress(packed []byte) ([]byte, error) {
	return zstdDec.DecodeAll(packed, nil)
}

// hashOf extracts the hex digest from a "sha256:<hex>" ref.
func hashOf(ref model.BlobRef) (string, error) {
	h := strings.TrimPrefix(string(ref), "sha256:")
	if len(h) < 2 {
		return "", errors.New("blob: malformed ref")
	}
	return h, nil
}

// FS is a filesystem-backed store. Layout: <root>/<hash[:2]>/<hash>, zstd body.
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
	h, err := hashOf(ref)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, h[:2], h), nil
}

// Put compresses and stores data, returning its content-addressed ref. It is
// idempotent: identical content is stored once.
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
	packed := compress(data)
	// Atomic publish via temp + rename.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, packed, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return ref, nil
}

// Get loads, decompresses, and integrity-checks the blob for ref. It returns
// ErrNotFound if ref is absent.
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
	out, err := decompress(raw)
	if err != nil {
		return nil, err
	}
	if Ref(out) != ref {
		return nil, errors.New("blob: integrity mismatch")
	}
	return out, nil
}

// Has reports whether ref is present.
// ListBlobs walks the store and returns every blob with its mod time.
func (s *FS) ListBlobs(_ context.Context) ([]BlobInfo, error) {
	var out []BlobInfo
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".tmp") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, BlobInfo{Ref: model.BlobRef("sha256:" + d.Name()), ModTime: info.ModTime()})
		return nil
	})
	return out, err
}

// Delete removes a blob (used by GC).
func (s *FS) Delete(_ context.Context, ref model.BlobRef) error {
	p, err := s.path(ref)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
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
