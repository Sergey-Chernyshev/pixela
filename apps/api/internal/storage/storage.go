// Package storage is the adapter for Pixela's object store (S3-compatible / MinIO). Screenshot and
// diff blobs are content-addressable: the object key is the sha256 over canonical decoded pixels, so
// identical images dedupe and an upload can be verified by re-hashing (invariant #4, see
// docs/architecture/go-backend.md §10.3). This file implements only the client lifecycle: connect,
// ensure the bucket, and report readiness via core.HealthChecker (§11.3). The CAS upload/presign
// methods are Phase 1 — see the TODO block at the bottom.
package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

const (
	// startupTimeout bounds the boot-time bucket-ensure. On failure we log and continue (do NOT fail
	// New) so the process boots and /readyz reports 503 rather than the container restart-looping on a
	// transient MinIO blip (readiness model, §11.3).
	startupTimeout = 5 * time.Second
	// healthCheckTimeout bounds each /readyz Check round-trip (§11.3: short per-check timeout).
	healthCheckTimeout = 2 * time.Second
)

// Compile-time guarantee that *Store satisfies the readiness contract.
var _ core.HealthChecker = (*Store)(nil)

// Config is the object-store connection configuration. Endpoint may include a scheme
// (http:// or https://) — New strips it and derives UseSSL accordingly (see New).
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// Store wraps a MinIO client bound to a single bucket. Construct it with New; it satisfies
// core.HealthChecker. The zero value is not usable.
type Store struct {
	client *minio.Client
	bucket string
	log    *slog.Logger
}

// New builds a MinIO client and ensures the target bucket exists.
//
// The endpoint passed to minio.New must be host:port WITHOUT a scheme. If cfg.Endpoint includes
// http:// or https://, New strips the scheme and derives Secure from it (https => true), overriding
// cfg.UseSSL for that case; a bare host:port uses cfg.UseSSL as-is.
//
// Bucket-ensure (BucketExists, then MakeBucket if absent) runs under a bounded context. If MinIO is
// unreachable at startup the failure is logged as a warning and New still returns a usable Store —
// readiness (GET /readyz) reports the outage instead of the process failing to boot. New fails only
// when the client itself cannot be constructed (malformed credentials/endpoint). log must be non-nil.
func New(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	endpoint, secure := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("new minio client: %w", err)
	}

	s := &Store{
		client: client,
		bucket: cfg.Bucket,
		log:    log,
	}

	if err := s.ensureBucket(ctx, cfg.Region); err != nil {
		log.WarnContext(ctx, "object store unreachable at startup; continuing (readiness will report it)",
			slog.String("bucket", cfg.Bucket),
			slog.String("error", err.Error()))
	}

	return s, nil
}

// normalizeEndpoint strips an optional scheme from a configured endpoint and returns the bare
// host:port plus the effective Secure flag. A bare host:port keeps the caller-supplied useSSL; a
// scheme-qualified endpoint overrides it (https => secure, http => insecure). This keeps the
// adapter robust to S3_ENDPOINT values like "http://minio:9000" or plain "minio:9000".
func normalizeEndpoint(endpoint string, useSSL bool) (host string, secure bool) {
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		return strings.TrimPrefix(endpoint, "https://"), true
	case strings.HasPrefix(endpoint, "http://"):
		return strings.TrimPrefix(endpoint, "http://"), false
	default:
		return endpoint, useSSL
	}
}

// ensureBucket creates the bucket if it does not already exist, under a bounded context so a hung
// MinIO cannot stall boot. Errors are wrapped with %w for errors.Is/As inspection by the caller.
func (s *Store) ensureBucket(ctx context.Context, region string) error {
	ensureCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	exists, err := s.client.BucketExists(ensureCtx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %q exists: %w", s.bucket, err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ensureCtx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		// A concurrent creator (another process booting) may win the race; treat an
		// already-owned/exists outcome as success rather than a startup warning.
		exists, existsErr := s.client.BucketExists(ensureCtx, s.bucket)
		if existsErr == nil && exists {
			return nil
		}
		return fmt.Errorf("make bucket %q: %w", s.bucket, errors.Join(err, existsErr))
	}

	s.log.InfoContext(ctx, "created object store bucket", slog.String("bucket", s.bucket))
	return nil
}

// Name implements core.HealthChecker; it identifies this dependency in the /readyz payload.
func (s *Store) Name() string { return "objectstore" }

// Check implements core.HealthChecker with a bounded reachability probe. It verifies the bucket is
// reachable via BucketExists; a non-nil error means the object store is down. The cause is wrapped
// with %w for errors.Is/As inspection.
func (s *Store) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()
	if _, err := s.client.BucketExists(checkCtx, s.bucket); err != nil {
		return fmt.Errorf("object store reachability: %w", err)
	}
	return nil
}

// TODO(phase-1): content-addressable storage methods. The diff engine and ingestion path land on
// these seams; they are intentionally NOT implemented here (Phase 0 ships only client + readiness +
// bucket-ensure, see docs/architecture/go-backend.md §14). When added, both MUST uphold invariant #4
// (keys = sha256 over canonical decoded pixels, §10.3):
//
//   - PutContentAddressed(ctx, sha256 string, data io.Reader, size int64, contentType string) error
//       Stores the blob at key=sha256 and VERIFIES the upload by re-hashing the decoded pixels
//       (never trust the byte stream); a mismatch maps to SNAPSHOT_HASH_MISMATCH. Idempotent: an
//       existing object with the same key is a no-op (CAS dedup).
//
//   - PresignGet(ctx, sha256 string, ttl time.Duration) (*url.URL, error)
//       Returns a time-limited presigned GET URL for the dashboard to fetch a blob directly.
//
// Keep encoding/hashing out of this adapter (it owns bytes, not pixels): the canonical sha256 is
// computed by the Encoder/CAS seam and passed in as the key.
