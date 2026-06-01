package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/dashboard"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/httpapi"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/ingestion"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/session"
)

const shutdownTimeout = 30 * time.Second

// runServe runs the HTTP API: health + the ingestion endpoints. The diff queue's insert-only client
// lets ingestion enqueue diff jobs transactionally on finalize.
func runServe(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	d, err := wire(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer d.close()

	q, err := queue.NewServeClient(d.db.Pool(), log)
	if err != nil {
		return fmt.Errorf("queue serve client: %w", err)
	}
	ingestSvc := ingestion.NewService(d.db, d.store, q, cfg.ImageMaxBytes, log)

	// Dashboard: server-side sessions in Redis (invariant: Redis is sessions-only), reads scoped to the
	// caller's memberships, presigned image URLs for the review viewer.
	sessions := session.NewStore(d.redis.Raw(), 0) // 0 → session.DefaultTTL
	dashSvc, err := dashboard.NewService(d.db, sessions, d.store, q,
		time.Duration(cfg.PresignedTTLSeconds)*time.Second, log)
	if err != nil {
		return fmt.Errorf("dashboard service: %w", err)
	}

	var ready atomic.Bool
	ready.Store(true) // adapters pinged in wire(); readiness reflects live checks from here on

	srv := httpapi.NewServer(httpapi.Deps{
		Logger:          log,
		Checkers:        d.checkers(),
		CORSOrigin:      cfg.CORSOrigin,
		Ready:           &ready,
		Ingestion:       ingestSvc,
		KeyResolver:     newKeyResolver(d.db, cfg.SessionSecret.Reveal(), log),
		Dashboard:       dashSvc,
		SessionResolver: newSessionResolver(d.db, sessions),
		SessionTTL:      sessions.TTL(),
		CookieSecure:    cfg.IsProduction(),
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("pixela serve listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down http server")
		// Fresh context — never the cancelled signal ctx (that would drop in-flight connections).
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		//nolint:contextcheck // graceful shutdown REQUIRES a fresh context, not the cancelled signal ctx (rulebook §7.6)
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}
}
