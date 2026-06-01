package diffrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diff"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
)

// diffWorker compares one snapshot against its baseline and writes the verdict. It is the only place
// the diff engine runs (invariant #3: never in an HTTP request).
type diffWorker struct {
	river.WorkerDefaults[queue.DiffJobArgs]
	deps Deps
}

// Work processes one diff job. A panic (e.g. a pathological image) is converted to an error so one bad
// job can never crash the worker process (rulebook §6, spec §07 error isolation).
func (w *diffWorker) Work(ctx context.Context, job *river.Job[queue.DiffJobArgs]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in diff worker: %v", r)
		}
	}()
	return w.process(ctx, job.Args.SnapshotID)
}

func (w *diffWorker) process(ctx context.Context, snapshotID string) error {
	q := w.deps.DB.Queries()

	snap, err := q.GetSnapshot(ctx, snapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // snapshot deleted; nothing to do
	}
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	if snap.Status != db.SnapshotStatusPENDING {
		return nil // already processed — idempotent on at-least-once delivery
	}

	build, err := q.GetBuild(ctx, snap.BuildID)
	if err != nil {
		return fmt.Errorf("get build: %w", err)
	}

	// Baseline resolution: strictly within the branch (NO merge-base; invariant #1).
	baseline, err := q.GetBaselineForKey(ctx, db.GetBaselineForKeyParams{
		ProjectID: build.ProjectID, Branch: build.Branch,
		Name: snap.Name, Browser: snap.Browser, Viewport: snap.Viewport,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// No baseline → NEW, no diff needed.
		return w.commit(ctx, snap.BuildID, func(qtx *db.Queries) error {
			return qtx.SetSnapshotNew(ctx, snapshotID)
		})
	}
	if err != nil {
		return fmt.Errorf("resolve baseline: %w", err)
	}

	if snap.NewImageSha == nil {
		return w.markError(ctx, snap.BuildID, snapshotID, "snapshot has no uploaded image")
	}

	// Download both blobs (transient errors here are retryable — return them to River).
	newBytes, err := w.deps.Store.GetBytes(ctx, *snap.NewImageSha)
	if err != nil {
		return fmt.Errorf("download new image: %w", err)
	}
	baseBytes, err := w.deps.Store.GetBytes(ctx, baseline.ImageSha)
	if err != nil {
		return fmt.Errorf("download baseline image: %w", err)
	}

	// Decode failures are deterministic (a corrupt PNG won't fix on retry) → ERROR, not retry.
	newImg, err := w.deps.Engine.Decode(bytes.NewReader(newBytes))
	if err != nil {
		return w.markError(ctx, snap.BuildID, snapshotID, "decode new image: "+err.Error())
	}
	baseImg, err := w.deps.Engine.Decode(bytes.NewReader(baseBytes))
	if err != nil {
		return w.markError(ctx, snap.BuildID, snapshotID, "decode baseline image: "+err.Error())
	}

	res, err := w.deps.Engine.Diff(baseImg, newImg, w.deps.Options)
	if err != nil {
		return w.markError(ctx, snap.BuildID, snapshotID, "diff: "+err.Error())
	}

	baselineID := baseline.ID
	if res.DiffPixels == 0 || res.DiffRatio <= w.deps.DiffRatioThreshold {
		return w.commit(ctx, snap.BuildID, func(qtx *db.Queries) error {
			return qtx.SetSnapshotUnchanged(ctx, db.SetSnapshotUnchangedParams{BaselineID: &baselineID, ID: snapshotID})
		})
	}

	// CHANGED: encode the diff image, content-address by decoded pixels, store it.
	var diffSha *string
	var diffPNG []byte
	var diffW, diffH int
	if res.DiffImage != nil {
		diffPNG, err = diff.EncodeDiffPNG(res.DiffImage)
		if err != nil {
			return w.markError(ctx, snap.BuildID, snapshotID, "encode diff: "+err.Error())
		}
		key := diff.ContentKey(res.DiffImage)
		if err := w.deps.Store.Put(ctx, key, diffPNG); err != nil {
			return fmt.Errorf("store diff image: %w", err) // transient → retry
		}
		b := res.DiffImage.Bounds()
		diffW, diffH = b.Dx(), b.Dy()
		diffSha = &key
	}

	ratio := res.DiffRatio
	pixels := int32(res.DiffPixels) //nolint:gosec // pixel count is bounded by image area
	return w.commit(ctx, snap.BuildID, func(qtx *db.Queries) error {
		if diffSha != nil {
			if err := qtx.UpsertImage(ctx, db.UpsertImageParams{
				//nolint:gosec // image dimensions and byte size are non-negative and bounded
				Sha256: *diffSha, Width: int32(diffW), Height: int32(diffH), ByteSize: int32(len(diffPNG)),
			}); err != nil {
				return err
			}
		}
		return qtx.SetSnapshotChanged(ctx, db.SetSnapshotChangedParams{
			DiffRatio: &ratio, DiffPixels: &pixels, DiffImageSha: diffSha, BaselineID: &baselineID, ID: snapshotID,
		})
	})
}

// commit applies the snapshot's terminal update and, if it was the last pending snapshot in the build,
// enqueues a build-finalize job — all in ONE transaction, so the state change and the enqueue commit
// atomically (no lost finalize, no premature finalize).
func (w *diffWorker) commit(ctx context.Context, buildID string, update func(qtx *db.Queries) error) error {
	tx, err := w.deps.DB.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin diff tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.deps.DB.Queries().WithTx(tx)

	if err := update(qtx); err != nil {
		return fmt.Errorf("update snapshot: %w", err)
	}
	pending, err := qtx.CountPendingSnapshots(ctx, buildID)
	if err != nil {
		return fmt.Errorf("count pending: %w", err)
	}
	if pending == 0 {
		if err := queue.EnqueueFinalizeBuildTx(ctx, tx, buildID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit diff: %w", err)
	}
	return nil
}

// markError records a per-snapshot ERROR (isolating the failure from the rest of the build) and still
// advances the build toward finalization. Returns nil on success so River does not retry a
// deterministic failure.
func (w *diffWorker) markError(ctx context.Context, buildID, snapshotID, msg string) error {
	w.deps.Log.WarnContext(ctx, "snapshot marked ERROR", "snapshot_id", snapshotID, "reason", msg)
	m := msg
	return w.commit(ctx, buildID, func(qtx *db.Queries) error {
		return qtx.SetSnapshotError(ctx, db.SetSnapshotErrorParams{ErrorMsg: &m, ID: snapshotID})
	})
}
