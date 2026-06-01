// Package ingestion implements the data-entry flow from CI: create a build, declare screenshots by
// hash (two-phase, content-addressed, idempotent), upload only the bytes that are new, then finalize
// (compute REMOVED, enqueue diff jobs). It owns no HTTP concerns — handlers live in httpapi and call
// this service, mapping its domain errors to the API envelope. See docs/spec/agents/02-ingestion-api.md.
package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// pngSignature is the 8-byte PNG magic header. We validate it rather than trusting Content-Type.
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Service orchestrates ingestion over the DB, object store and queue.
type Service struct {
	db            *db.DB
	store         *storage.Store
	queue         *queue.Queue
	log           *slog.Logger
	imageMaxBytes int64
}

// NewService wires the ingestion service. imageMaxBytes caps an uploaded PNG (IMAGE_TOO_LARGE).
func NewService(database *db.DB, store *storage.Store, q *queue.Queue, imageMaxBytes int64, log *slog.Logger) *Service {
	return &Service{db: database, store: store, queue: q, log: log, imageMaxBytes: imageMaxBytes}
}

// CreateBuildInput is the metadata for a new build (the key's project is supplied separately).
type CreateBuildInput struct {
	Branch    string
	CommitSha string
	CIBuildID *string
	CIJobURL  *string
	MRIID     *string
}

// CreateBuild starts a RUNNING build owned by the API key's project.
func (s *Service) CreateBuild(ctx context.Context, projectID string, in CreateBuildInput) (db.Build, error) {
	build, err := s.db.Queries().CreateBuild(ctx, db.CreateBuildParams{
		ID:        core.NewID(),
		ProjectID: projectID,
		Branch:    in.Branch,
		CommitSha: in.CommitSha,
		CiBuildID: in.CIBuildID,
		CiJobUrl:  in.CIJobURL,
		MrIid:     in.MRIID,
	})
	if err != nil {
		return db.Build{}, fmt.Errorf("create build: %w", err)
	}
	return build, nil
}

// DeclareSnapshotInput is step 1: declare a screenshot by its client-computed sha256.
type DeclareSnapshotInput struct {
	Name        string
	Browser     string
	Viewport    string
	ImageSha256 string
	Width       int32
	Height      int32
	ByteSize    int32
	// BaselinePath is the repo-relative path of this snapshot's baseline file (Mode A); optional. On
	// approve, Pixela commits the new image to this path on the build's branch.
	BaselinePath *string
}

// DeclareSnapshot performs the two-phase upload's step 1: idempotently upsert the snapshot row and
// report whether the blob bytes still need uploading (needUpload=false ⇒ a blob with this sha already
// exists in the store, the core of CAS dedup). Image metadata is upserted first as the FK target.
func (s *Service) DeclareSnapshot(ctx context.Context, projectID, buildID string, in DeclareSnapshotInput) (snapshotID string, needUpload bool, err error) {
	if err = s.assertOwnedRunningBuild(ctx, projectID, buildID); err != nil {
		return "", false, err
	}

	if err = s.db.Queries().UpsertImage(ctx, db.UpsertImageParams{
		Sha256:   in.ImageSha256,
		Width:    in.Width,
		Height:   in.Height,
		ByteSize: in.ByteSize,
	}); err != nil {
		return "", false, fmt.Errorf("upsert image metadata: %w", err)
	}

	sha := in.ImageSha256
	snap, err := s.db.Queries().UpsertSnapshot(ctx, db.UpsertSnapshotParams{
		ID:           core.NewID(),
		BuildID:      buildID,
		Name:         in.Name,
		Browser:      in.Browser,
		Viewport:     in.Viewport,
		NewImageSha:  &sha,
		BaselinePath: in.BaselinePath,
	})
	if err != nil {
		return "", false, fmt.Errorf("upsert snapshot: %w", err)
	}

	exists, err := s.store.Exists(ctx, in.ImageSha256)
	if err != nil {
		return "", false, fmt.Errorf("check blob existence: %w", err)
	}
	return snap.ID, !exists, nil
}

// UploadImage performs step 2: validate the bytes (size, PNG magic, sha integrity) and store them.
// The image is global/content-addressed, so any valid key may upload it; the HTTP layer enforces auth.
func (s *Service) UploadImage(ctx context.Context, declaredSha string, data []byte) error {
	if int64(len(data)) > s.imageMaxBytes {
		return fmt.Errorf("image is %d bytes (max %d): %w", len(data), s.imageMaxBytes, core.ErrImageTooLarge)
	}
	if !bytes.HasPrefix(data, pngSignature) {
		return fmt.Errorf("not a PNG (bad magic bytes): %w", core.ErrValidation)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != declaredSha {
		return fmt.Errorf("uploaded bytes do not match declared sha256: %w", core.ErrHashMismatch)
	}
	if err := s.store.Put(ctx, declaredSha, data); err != nil {
		return fmt.Errorf("store blob: %w", err)
	}
	return nil
}

// FinalizeBuild closes the RUNNING build: under one transaction it computes REMOVED snapshots
// (baselines with no matching snapshot in the build), flips the build to COMPARING, and enqueues a
// diff job per pending snapshot — atomically (InsertTx), so jobs exist iff the state change commits.
func (s *Service) FinalizeBuild(ctx context.Context, projectID, buildID string) (db.Build, error) {
	if err := s.assertOwnedRunningBuild(ctx, projectID, buildID); err != nil {
		return db.Build{}, err
	}

	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return db.Build{}, fmt.Errorf("begin finalize tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.db.Queries().WithTx(tx)

	locked, err := qtx.GetBuildForUpdate(ctx, buildID)
	if err != nil {
		return db.Build{}, fmt.Errorf("lock build: %w", err)
	}
	if locked.Status != db.BuildStatusRUNNING {
		return db.Build{}, fmt.Errorf("build %s already finalized: %w", buildID, core.ErrBuildFinalized)
	}

	missing, err := qtx.ListMissingBaselines(ctx, db.ListMissingBaselinesParams{
		ProjectID: projectID,
		Branch:    locked.Branch,
		BuildID:   buildID,
	})
	if err != nil {
		return db.Build{}, fmt.Errorf("list missing baselines: %w", err)
	}
	for _, m := range missing {
		baselineID := m.ID
		if err := qtx.InsertRemovedSnapshot(ctx, db.InsertRemovedSnapshotParams{
			ID:         core.NewID(),
			BuildID:    buildID,
			Name:       m.Name,
			Browser:    m.Browser,
			Viewport:   m.Viewport,
			BaselineID: &baselineID,
		}); err != nil {
			return db.Build{}, fmt.Errorf("insert removed snapshot: %w", err)
		}
	}

	if err := qtx.SetBuildComparing(ctx, buildID); err != nil {
		return db.Build{}, fmt.Errorf("set build comparing: %w", err)
	}

	pending, err := qtx.ListPendingSnapshotIDs(ctx, buildID)
	if err != nil {
		return db.Build{}, fmt.Errorf("list pending snapshots: %w", err)
	}
	// We enqueue a diff job per pending snapshot without re-asserting its blob exists (a client could,
	// buggily, skip the upload after a needUpload:false). The Phase-2 diff worker is responsible for
	// treating a missing blob as a per-snapshot ERROR (not a process crash) — spec §07 error isolation.
	if err := s.queue.EnqueueDiffJobs(ctx, tx, pending); err != nil {
		return db.Build{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return db.Build{}, fmt.Errorf("commit finalize: %w", err)
	}

	build, err := s.db.Queries().GetBuild(ctx, buildID)
	if err != nil {
		return db.Build{}, fmt.Errorf("reload finalized build: %w", err)
	}
	return build, nil
}

// assertOwnedRunningBuild enforces existence, project isolation (invariant #5) and that the build is
// still accepting screenshots.
func (s *Service) assertOwnedRunningBuild(ctx context.Context, projectID, buildID string) error {
	b, err := s.db.Queries().GetBuild(ctx, buildID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("build %s: %w", buildID, core.ErrBuildNotFound)
	}
	if err != nil {
		return fmt.Errorf("get build: %w", err)
	}
	if b.ProjectID != projectID {
		return fmt.Errorf("build %s not in project: %w", buildID, core.ErrForbiddenProject)
	}
	if b.Status != db.BuildStatusRUNNING {
		return fmt.Errorf("build %s already finalized: %w", buildID, core.ErrBuildFinalized)
	}
	return nil
}
