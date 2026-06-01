package diffrun

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
)

// finalizeWorker recomputes a build's aggregate status once all snapshots are terminal: PASSED if
// nothing needs review, else REVIEW_REQUIRED. A single ERROR snapshot moves the build to
// REVIEW_REQUIRED, never to a whole-build ERROR (spec §07). Idempotent: re-running on an
// already-finalized build is a no-op.
type finalizeWorker struct {
	river.WorkerDefaults[queue.FinalizeBuildArgs]
	db  *db.DB
	log *slog.Logger
}

func (w *finalizeWorker) Work(ctx context.Context, job *river.Job[queue.FinalizeBuildArgs]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in finalize worker: %v", r)
		}
	}()

	buildID := job.Args.BuildID
	tx, err := w.db.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finalize tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.db.Queries().WithTx(tx)

	build, err := qtx.GetBuildForUpdate(ctx, buildID)
	if err != nil {
		return fmt.Errorf("lock build: %w", err)
	}
	if build.Status != db.BuildStatusCOMPARING {
		return nil // already finalized — idempotent
	}

	reviewable, err := qtx.CountReviewableSnapshots(ctx, buildID)
	if err != nil {
		return fmt.Errorf("count reviewable: %w", err)
	}
	status := db.BuildStatusPASSED
	if reviewable > 0 {
		status = db.BuildStatusREVIEWREQUIRED
	}
	if err := qtx.SetBuildStatus(ctx, db.SetBuildStatusParams{Status: status, ID: buildID}); err != nil {
		return fmt.Errorf("set build status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finalize: %w", err)
	}

	w.log.InfoContext(ctx, "build status recomputed", "build_id", buildID, "status", status, "reviewable", reviewable)
	return nil
}
