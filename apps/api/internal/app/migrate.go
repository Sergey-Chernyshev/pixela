package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	dbassets "github.com/Sergey-Chernyshev/pixela/apps/api/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
)

// runMigrate brings a clean database up to the current schema and installs River's tables.
//
// Phase 0 applies the embedded canonical schema (db/schema.sql) transactionally if absent — keeping
// the binary self-contained (no atlas runtime dependency, distroless-friendly). Atlas remains the
// authoring + CI-lint authority; versioned Atlas migrations take over from the first schema change
// (Phase 1+). See docs/architecture/go-backend.md §8.4.
func runMigrate(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL.Reveal())
	if err != nil {
		return fmt.Errorf("connect for migrate: %w", err)
	}
	defer pool.Close()

	if err := applySchema(ctx, pool, log); err != nil {
		return err
	}
	if err := applyIncrementalMigrations(ctx, pool, log); err != nil {
		return err
	}
	if err := queue.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}
	log.Info("migrations applied")
	return nil
}

// incrementalMigrations are idempotent, additive DDL statements applied on EVERY migrate (after the
// initial schema). They bring an already-initialized database forward without a full Atlas runtime —
// each must be safe to re-run (ADD COLUMN IF NOT EXISTS, etc.). Append-only; never edit/remove past
// entries. See docs/architecture/go-backend.md §8.4.
var incrementalMigrations = []string{
	// Phase 5 (Mode A git-native): per-project GitLab repo + per-snapshot baseline file path.
	`ALTER TABLE projects ADD COLUMN IF NOT EXISTS gitlab_project_id TEXT`,
	`ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS baseline_path TEXT`,
}

func applyIncrementalMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	for i, stmt := range incrementalMigrations {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("incremental migration %d (%q): %w", i, stmt, err)
		}
	}
	log.Info("incremental migrations applied", "count", len(incrementalMigrations))
	return nil
}

// applySchema applies the embedded schema once, transactionally. Idempotent: a presence check on a
// sentinel table makes re-runs a no-op (and avoids any "dirty" half-applied state).
func applySchema(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	const sentinel = `SELECT EXISTS (
		SELECT FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'projects'
	)`
	var exists bool
	if err := pool.QueryRow(ctx, sentinel).Scan(&exists); err != nil {
		return fmt.Errorf("probe schema: %w", err)
	}
	if exists {
		log.Info("app schema already present, skipping")
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	if _, err := tx.Exec(ctx, dbassets.SchemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema: %w", err)
	}
	log.Info("app schema applied")
	return nil
}
