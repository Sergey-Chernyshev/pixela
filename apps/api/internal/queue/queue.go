// Package queue wraps the River (Postgres-transactional) job queue behind a
// small API. River shares the same pgx pool as the rest of the application;
// Redis is not used for the queue. The serve process inserts jobs only, while
// the worker process registers workers and runs them.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// HealthCheckArgs is a placeholder job kind so the worker has something
// registered and the queue design is exercised end to end. Later phases add the
// real DiffJob / FinalizeBuildJob kinds.
type HealthCheckArgs struct{}

// Kind uniquely identifies the job type for River across deploys.
func (HealthCheckArgs) Kind() string { return "pixela.healthcheck" }

// healthCheckWorker handles HealthCheckArgs jobs.
type healthCheckWorker struct {
	river.WorkerDefaults[HealthCheckArgs]
	log *slog.Logger
}

// Work runs a healthcheck job. The deferred recover converts a panic into an
// error so a misbehaving job can never crash the worker process (rulebook §6).
func (w *healthCheckWorker) Work(ctx context.Context, job *river.Job[HealthCheckArgs]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in healthcheck worker: %v", r)
		}
	}()

	w.log.InfoContext(ctx, "river healthcheck job", "id", job.ID)
	return nil
}

// Queue wraps the generic River client behind a small, non-generic API so the
// rest of the application does not depend on river.Client[pgx.Tx] directly.
type Queue struct {
	client *river.Client[pgx.Tx]
	log    *slog.Logger
}

// NewServeClient builds an insert-only River client for the serve process: no
// Queues and no Workers are registered, and Start is never called. It exists so
// the API can enqueue jobs (via InsertTx in later phases) inside the same
// transaction as the work that produced them.
func NewServeClient(pool *pgxpool.Pool, log *slog.Logger) (*Queue, error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: log,
	})
	if err != nil {
		return nil, fmt.Errorf("new serve river client: %w", err)
	}

	return &Queue{client: client, log: log}, nil
}

// NewWorkerClient builds a full River client for the worker process: it
// registers the workers bundle and a default queue sized to the available CPUs
// (diff work is CPU-bound; job-level concurrency is River's, not ours — see
// rulebook §6). Call Start to begin processing.
func NewWorkerClient(pool *pgxpool.Pool, log *slog.Logger) (*Queue, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &healthCheckWorker{log: log})
	river.AddWorker(workers, &diffWorker{log: log})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: log,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: runtime.NumCPU()},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("new worker river client: %w", err)
	}

	return &Queue{client: client, log: log}, nil
}

// Start begins fetching and working jobs. It is only meaningful on a client
// built with NewWorkerClient.
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("start river client: %w", err)
	}
	return nil
}

// Stop gracefully drains in-flight jobs and shuts the client down. The given
// context bounds how long the drain may take.
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		return fmt.Errorf("stop river client: %w", err)
	}
	return nil
}

// Client exposes the underlying River client for transactional inserts
// (InsertTx) in later phases. Prefer the wrapper methods where possible.
func (q *Queue) Client() *river.Client[pgx.Tx] { return q.client }

// Migrate runs River's schema migrations up against the given pool, creating
// the river_job and related tables the queue needs to operate.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("new river migrator: %w", err)
	}

	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("run river migrations: %w", err)
	}

	return nil
}
