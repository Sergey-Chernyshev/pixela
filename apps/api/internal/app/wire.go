package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	pxredis "github.com/Sergey-Chernyshev/pixela/apps/api/internal/redis"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// deps holds the shared adapters used by both serve and worker. Built once by wire().
type deps struct {
	db    *db.DB
	redis *pxredis.Client
	store *storage.Store
}

// wire opens every adapter in dependency order, rolling back partial construction on failure so a
// failed boot leaks nothing.
func wire(ctx context.Context, cfg config.Config, log *slog.Logger) (*deps, error) {
	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	rc, err := pxredis.New(ctx, cfg.RedisURL.Reveal(), log)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("open redis: %w", err)
	}

	store, err := storage.New(ctx, storage.Config{
		Endpoint:       cfg.S3Endpoint,
		PublicEndpoint: cfg.S3PublicEndpoint,
		Region:         cfg.S3Region,
		Bucket:         cfg.S3Bucket,
		AccessKey:      cfg.S3AccessKey.Reveal(),
		SecretKey:      cfg.S3SecretKey.Reveal(),
		UseSSL:         cfg.S3UseSSL,
	}, log)
	if err != nil {
		_ = rc.Close()
		database.Close()
		return nil, fmt.Errorf("open storage: %w", err)
	}

	return &deps{db: database, redis: rc, store: store}, nil
}

// checkers is the readiness set exposed at GET /readyz.
func (d *deps) checkers() []core.HealthChecker {
	return []core.HealthChecker{d.db, d.redis, d.store}
}

// close releases adapters in reverse order. Safe to defer.
func (d *deps) close() {
	if d == nil {
		return
	}
	if d.redis != nil {
		_ = d.redis.Close()
	}
	if d.db != nil {
		d.db.Close()
	}
}
