package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// DiffJobArgs asks the worker to compare one snapshot against its baseline. Enqueued by ingestion on
// build finalize, in the SAME transaction as the snapshot rows (InsertTx) so a job exists iff its
// snapshot committed — no lost/phantom jobs. The real comparison lands in Phase 2.
type DiffJobArgs struct {
	SnapshotID string `json:"snapshot_id"`
}

// Kind uniquely identifies the job type for River across deploys.
func (DiffJobArgs) Kind() string { return "pixela.diff" }

// InsertOpts dedupes by args so a CI retry that re-enqueues the same snapshot's diff is a no-op
// (at-least-once-safe).
func (DiffJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// diffWorker is a Phase-0/1 stub: it acknowledges the job without comparing. The real pure-Go
// pixelmatch implementation lands in Phase 2. The deferred recover keeps a panic from crashing the
// worker process (rulebook §6).
type diffWorker struct {
	river.WorkerDefaults[DiffJobArgs]
	log *slog.Logger
}

func (w *diffWorker) Work(ctx context.Context, job *river.Job[DiffJobArgs]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in diff worker: %v", r)
		}
	}()
	w.log.InfoContext(ctx, "diff job received (stub; comparison lands in Phase 2)",
		"job_id", job.ID, "snapshot_id", job.Args.SnapshotID)
	return nil
}

// EnqueueDiffJobs inserts one diff job per snapshot id within the given transaction (InsertManyTx).
// Call it inside the same tx that finalizes the build so enqueue and state change commit atomically.
func (q *Queue) EnqueueDiffJobs(ctx context.Context, tx pgx.Tx, snapshotIDs []string) error {
	if len(snapshotIDs) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, 0, len(snapshotIDs))
	for _, id := range snapshotIDs {
		params = append(params, river.InsertManyParams{Args: DiffJobArgs{SnapshotID: id}})
	}
	if _, err := q.client.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("enqueue %d diff jobs: %w", len(snapshotIDs), err)
	}
	return nil
}
