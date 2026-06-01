package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diff"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diffrun"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/gitlab"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/gitsync"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
)

// runWorker runs the River diff-job consumer. Requires `pixela migrate` to have created River's tables.
func runWorker(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	d, err := wire(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer d.close()

	workers := river.NewWorkers()
	diffrun.AddWorkers(workers, diffrun.Deps{
		DB:                 d.db,
		Store:              d.store,
		Engine:             diff.NewStdlibEngine(),
		Log:                log,
		Options:            diff.DefaultOptions(),
		DiffRatioThreshold: 0, // any changed pixel => CHANGED (per-project override is a later refinement)
	})
	// Git-native side effects (Mode A): commit approved baselines + mirror build status to GitLab.
	gitsync.AddWorkers(workers, gitsync.Deps{
		DB:        d.db,
		Store:     d.store,
		GitLab:    gitlab.New(cfg.GitLabBaseURL, cfg.GitLabToken.Reveal()),
		Log:       log,
		PublicURL: cfg.PublicURL,
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
