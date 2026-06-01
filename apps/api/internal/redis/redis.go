// Package redis is the adapter for Pixela's Redis dependency. Redis backs dashboard sessions ONLY
// — it is NOT a queue (River on Postgres owns the queue; see docs/architecture/go-backend.md §1.1,
// §9). The package implements core.HealthChecker so GET /readyz can report Redis liveness (§11.3).
// The go-redis import is aliased goredis to avoid clashing with this package's own name.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	// startupPingTimeout bounds the boot-time Ping. On failure we log and continue (do NOT fail New)
	// so the process boots and /readyz reports 503 rather than the container restart-looping on a
	// transient Redis blip (readiness model, §11.3).
	startupPingTimeout = 3 * time.Second
	// healthPingTimeout bounds each /readyz Check round-trip (§11.3: short per-check timeout).
	healthPingTimeout = 2 * time.Second
	// maxRetries is the per-command retry budget for transient failures.
	maxRetries = 2
)

// Client wraps a *goredis.Client used for dashboard sessions. Construct it with New; it satisfies
// core.HealthChecker. The zero value is not usable.
type Client struct {
	rdb *goredis.Client
	log *slog.Logger
}

// New parses url, builds a Redis client with sane defaults, and attempts a bounded startup Ping.
// A failed Ping is logged as a warning and does NOT fail New — the client is returned regardless so
// readiness (GET /readyz) reports the outage instead of the process failing to boot. New fails only
// when url itself is malformed. log must be non-nil.
func New(ctx context.Context, url string, log *slog.Logger) (*Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	opts.MaxRetries = maxRetries

	c := &Client{
		rdb: goredis.NewClient(opts),
		log: log,
	}

	pingCtx, cancel := context.WithTimeout(ctx, startupPingTimeout)
	defer cancel()
	if err := c.rdb.Ping(pingCtx).Err(); err != nil {
		log.WarnContext(ctx, "redis unreachable at startup; continuing (readiness will report it)",
			slog.String("error", err.Error()))
	}

	return c, nil
}

// Name implements core.HealthChecker; it identifies this dependency in the /readyz payload.
func (c *Client) Name() string { return "redis" }

// Check implements core.HealthChecker with a bounded PING round-trip. A non-nil error means Redis is
// down; the cause is wrapped with %w for errors.Is/As inspection.
func (c *Client) Check(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, healthPingTimeout)
	defer cancel()
	if err := c.rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Raw returns the underlying go-redis client for wiring the session store (later phases). Callers
// must not close it; use Close on this wrapper instead.
func (c *Client) Raw() *goredis.Client { return c.rdb }

// Close releases the connection pool. It is safe to call once during shutdown.
func (c *Client) Close() error { return c.rdb.Close() }
