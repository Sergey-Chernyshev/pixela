// Package db is the Postgres data layer: the sqlc-generated typed queries (models.go, query.sql.go)
// plus this hand-written pgxpool wrapper. It depends on internal/core only (rulebook §2 import DAG);
// domain types live in core, never here. See docs/architecture/go-backend.md §8.1/§8.3.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool tuning constants (rulebook §8.1). Conservative for a self-hosted ~50-project workload.
const (
	poolMaxConns          = 20
	poolMinConns          = 2
	poolMaxConnLifetime   = time.Hour
	poolMaxConnLifeJitter = 5 * time.Minute
	poolMaxConnIdleTime   = 30 * time.Minute
	poolHealthCheckPeriod = time.Minute
	healthCheckTimeout    = 2 * time.Second
)

// DB owns the process-wide pgx connection pool and the sqlc-generated query set bound to it.
// One DB per process. It implements core.HealthChecker for the GET /readyz probe.
type DB struct {
	pool *pgxpool.Pool
	q    *Queries
	log  *slog.Logger
}

// Open parses the DSN, applies pool tuning, opens the pool, and verifies connectivity with an
// immediate Ping (pgxpool returns before any connection is established, so the Ping is what proves
// the database is reachable at boot). The returned *DB carries a *Queries bound to the pool.
//
// Named Open, not New, because sqlc generates a New(DBTX) *Queries constructor into this same
// package; two package-level New funcs would not compile.
func Open(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.MaxConnLifetimeJitter = poolMaxConnLifeJitter
	cfg.MaxConnIdleTime = poolMaxConnIdleTime
	cfg.HealthCheckPeriod = poolHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("database pool ready", "max_conns", cfg.MaxConns, "min_conns", cfg.MinConns)

	// pgxpool.Pool satisfies the sqlc-generated DBTX interface, so New(pool) binds the query set.
	return &DB{pool: pool, q: New(pool), log: log}, nil
}

// Name identifies this dependency in the /readyz payload (core.HealthChecker).
func (d *DB) Name() string { return "database" }

// Check performs a bounded round-trip to Postgres for the readiness probe (core.HealthChecker).
func (d *DB) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	if err := d.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// Pool exposes the underlying pgxpool for callers that need raw access (e.g. River's pgx driver).
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// Queries exposes the sqlc-generated query set bound to the pool.
func (d *DB) Queries() *Queries { return d.q }

// Stat returns pool statistics for the /readyz payload (rulebook §8.1).
func (d *DB) Stat() *pgxpool.Stat { return d.pool.Stat() }

// Close releases all pool connections. Call once at shutdown.
func (d *DB) Close() { d.pool.Close() }

// ExecTx runs fn inside a single transaction, passing it a *Queries bound to that transaction.
// On any error (from fn or commit) the transaction is rolled back. pgx does NOT auto-rollback on
// context cancellation (unlike database/sql), so the deferred Rollback is mandatory — see rulebook
// §8.3. The deferred Rollback after a successful Commit is a documented no-op returning
// pgx.ErrTxClosed, which is ignored.
func (d *DB) ExecTx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			d.log.ErrorContext(ctx, "rollback failed", "error", rbErr)
		}
	}()

	if err := fn(d.q.WithTx(tx)); err != nil {
		return fmt.Errorf("exec tx: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
