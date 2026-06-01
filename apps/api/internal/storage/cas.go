package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	minio "github.com/minio/minio-go/v7"
)

// pngContentType is the stored content type. Object keys are the sha256 of the canonical decoded
// pixels (invariant #4); the same image therefore dedupes to one object.

// Exists reports whether a blob with this key is already stored — the basis of upload dedup
// (needUpload). A NoSuchKey response is a clean false, not an error.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat object %q: %w", key, err)
	}
	return true, nil
}

// Put stores data under key, idempotently: if the blob already exists it is a no-op (content is
// addressed by key, so identical key implies identical bytes). Safe to call on a CI retry.
func (s *Store) Put(ctx context.Context, key string, data []byte) error {
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "image/png"})
	if err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

// PresignedGetURL returns a short-lived URL the dashboard can use to read a blob directly, without
// the API proxying bytes and without exposing object-store credentials (rulebook §7; spec §10).
func (s *Store) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign get %q: %w", key, err)
	}
	return u.String(), nil
}

// GetBytes downloads the blob bytes (used by the diff worker in Phase 2).
func (s *Store) GetBytes(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	defer func() { _ = obj.Close() }()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", key, err)
	}
	return data, nil
}
