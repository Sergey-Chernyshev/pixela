package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diff"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diffrun"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
)

// runWorker runs the River diff-job consumer. Requires `pixela migrate` to have created River's tables.
func runWorker(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	d, err := wire(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer d.close()

	workers := diffrun.Workers(diffrun.Deps{
		DB:                 d.db,
		Store:              d.store,
		Engine:             diff.NewStdlibEngine(),
		Log:                log,
		Options:            diff.DefaultOptions(),
		DiffRatioThreshold: 0, // any changed pixel => CHANGED (per-project override is a later refinement)
	})
	q, err := queue.NewWorkerClient(d.db.Pool(), log, workers)
	if err != nil {
		return fmt.Errorf("queue worker client: %w", err)
	}
	if err := q.Start(ctx); err != nil {
		return fmt.Errorf("start queue: %w", err)
	}
	log.Info("pixela worker started")

	<-ctx.Done()
	log.Info("shutting down worker")
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	//nolint:contextcheck // graceful drain REQUIRES a fresh context, not the cancelled signal ctx (rulebook §6)
	if err := q.Stop(stopCtx); err != nil {
		return fmt.Errorf("stop queue: %w", err)
	}
	return nil
}
