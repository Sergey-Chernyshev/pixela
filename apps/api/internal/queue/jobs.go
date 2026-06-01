package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// DiffJobArgs asks the worker to compare one snapshot against its baseline. Enqueued by ingestion on
// build finalize, in the SAME transaction as the snapshot rows (InsertTx) so a job exists iff its
// snapshot committed. Worker impl lives in internal/diffrun (Phase 2).
type DiffJobArgs struct {
	SnapshotID string `json:"snapshot_id"`
}

// Kind uniquely identifies the job type for River across deploys.
func (DiffJobArgs) Kind() string { return "pixela.diff" }

// InsertOpts dedupes by args so a CI retry that re-enqueues the same snapshot's diff is a no-op.
func (DiffJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// FinalizeBuildArgs recomputes a build's aggregate status once all its snapshots are terminal. The
// diff job that observes the last pending snapshot enqueues this, unique by build, in its own tx.
type FinalizeBuildArgs struct {
	BuildID string `json:"build_id"`
}

// Kind uniquely identifies the job type for River across deploys.
func (FinalizeBuildArgs) Kind() string { return "pixela.finalize_build" }

// InsertOpts dedupes by build so concurrent diff jobs that all observe "0 pending" enqueue once.
func (FinalizeBuildArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
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

// EnqueueFinalizeBuildTx enqueues a build-finalize job inside tx. It is meant to be called from within
// a River worker, where the executing client is available via the context (avoiding a chicken-and-egg
// dependency between the worker bundle and the client).
func EnqueueFinalizeBuildTx(ctx context.Context, tx pgx.Tx, buildID string) error {
	client := river.ClientFromContext[pgx.Tx](ctx)
	if client == nil {
		return errors.New("no river client in context")
	}
	if _, err := client.InsertTx(ctx, tx, FinalizeBuildArgs{BuildID: buildID}, nil); err != nil {
		return fmt.Errorf("enqueue finalize build %s: %w", buildID, err)
	}
	return nil
}
