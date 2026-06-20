package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vul-os/vmail/internal/model"
)

// S3 is an S3-compatible (minio, AWS S3, etc.) Store. Objects are zstd-compressed
// and keyed by content hash, so Put is idempotent and bodies dedup globally. The
// stored format is internal and integrity-checked on read.
type S3 struct {
	cli    *minio.Client
	bucket string
}

// NewS3 connects to an S3-compatible endpoint and returns a Store backed by
// bucket, creating the bucket if it does not already exist.
func NewS3(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*S3, error) {
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &S3{cli: cli, bucket: bucket}, nil
}

// objectKey derives the object key for a ref: <hash[:2]>/<hash>.
func objectKey(ref model.BlobRef) (string, error) {
	h, err := hashOf(ref)
	if err != nil {
		return "", err
	}
	return h[:2] + "/" + h, nil
}

// isNotFound reports whether err is a minio "object/key absent" error.
func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.StatusCode == http.StatusNotFound ||
		resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket"
}

// Put compresses and stores data, returning its content-addressed ref. It is
// idempotent: if the object already exists it is left untouched.
func (s *S3) Put(ctx context.Context, data []byte) (model.BlobRef, error) {
	ref := Ref(data)
	key, err := objectKey(ref)
	if err != nil {
		return "", err
	}
	// Idempotency / dedup: skip the upload if the object is already present.
	if _, err := s.cli.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err == nil {
		return ref, nil
	} else if !isNotFound(err) {
		return "", err
	}
	packed := compress(data)
	if _, err := s.cli.PutObject(ctx, s.bucket, key, bytes.NewReader(packed),
		int64(len(packed)), minio.PutObjectOptions{ContentType: "application/zstd"}); err != nil {
		return "", err
	}
	return ref, nil
}

// Get loads, decompresses, and integrity-checks the blob for ref. It returns
// ErrNotFound if ref is absent.
func (s *S3) Get(ctx context.Context, ref model.BlobRef) ([]byte, error) {
	key, err := objectKey(ref)
	if err != nil {
		return nil, err
	}
	obj, err := s.cli.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	packed, err := io.ReadAll(obj)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out, err := decompress(packed)
	if err != nil {
		return nil, err
	}
	if Ref(out) != ref {
		return nil, errors.New("blob: integrity mismatch")
	}
	return out, nil
}

// Has reports whether ref is present.
func (s *S3) Has(ctx context.Context, ref model.BlobRef) (bool, error) {
	key, err := objectKey(ref)
	if err != nil {
		return false, err
	}
	if _, err := s.cli.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Compile-time assertion that S3 satisfies the Store contract.
var _ Store = (*S3)(nil)
