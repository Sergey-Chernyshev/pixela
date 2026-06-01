package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// GitCommitFile is one file change in a Mode A baseline commit (action chosen from the snapshot status:
// NEW→create, CHANGED→update, REMOVED→delete). ImageSha is the blob to fetch from the object store
// (empty for delete).
type GitCommitFile struct {
	Action   string `json:"action"`
	Path     string `json:"path"`
	ImageSha string `json:"image_sha"`
}

// GitCommitJobArgs commits approved baseline changes back to the project's GitLab repo on the build's
// branch (git-native baseline, invariant #1). Enqueued in the same tx as the approve.
type GitCommitJobArgs struct {
	BuildID string          `json:"build_id"`
	UserID  string          `json:"user_id"`
	Files   []GitCommitFile `json:"files"`
}

// Kind uniquely identifies the job type for River across deploys.
func (GitCommitJobArgs) Kind() string { return "pixela.git_commit" }

// GitStatusJobArgs mirrors a build's review state onto its GitLab commit/MR. Deduped by build — the
// worker reads the build's CURRENT status at run time, so coalescing concurrent enqueues is correct.
type GitStatusJobArgs struct {
	BuildID string `json:"build_id"`
}

// Kind uniquely identifies the job type for River across deploys.
func (GitStatusJobArgs) Kind() string { return "pixela.git_status" }

// InsertOpts dedupes by build so a flurry of status changes coalesces into one pending mirror job.
func (GitStatusJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// EnqueueGitCommitTx enqueues a baseline-commit job inside tx, so it exists iff the approve commits.
// A no-op when there are no file changes to commit.
func (q *Queue) EnqueueGitCommitTx(ctx context.Context, tx pgx.Tx, args GitCommitJobArgs) error {
	if len(args.Files) == 0 {
		return nil
	}
	if _, err := q.client.InsertTx(ctx, tx, args, nil); err != nil {
		return fmt.Errorf("enqueue git commit: %w", err)
	}
	return nil
}

// EnqueueGitStatusTx enqueues a build-status mirror job inside tx (serve process: the queue's own client).
func (q *Queue) EnqueueGitStatusTx(ctx context.Context, tx pgx.Tx, buildID string) error {
	if _, err := q.client.InsertTx(ctx, tx, GitStatusJobArgs{BuildID: buildID}, nil); err != nil {
		return fmt.Errorf("enqueue git status: %w", err)
	}
	return nil
}

// EnqueueGitStatusFromContextTx enqueues a build-status mirror job from WITHIN a River worker (the
// executing client is taken from the context), e.g. when the diff finalize sets the first review state.
func EnqueueGitStatusFromContextTx(ctx context.Context, tx pgx.Tx, buildID string) error {
	client := river.ClientFromContext[pgx.Tx](ctx)
	if client == nil {
		return errors.New("no river client in context")
	}
	if _, err := client.InsertTx(ctx, tx, GitStatusJobArgs{BuildID: buildID}, nil); err != nil {
		return fmt.Errorf("enqueue git status %s: %w", buildID, err)
	}
	return nil
}
