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
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/httpapi"
)

const shutdownTimeout = 30 * time.Second

// runServe runs the HTTP API. The diff queue's insert-only client is wired here in Phase 1 (when
// ingestion enqueues jobs); Phase 0 serves health + the (empty) OpenAPI surface.
func runServe(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	d, err := wire(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer d.close()

	var ready atomic.Bool
	ready.Store(true) // adapters pinged in wire(); readiness reflects live checks from here on

	srv := httpapi.NewServer(httpapi.Deps{
		Logger:     log,
		Checkers:   d.checkers(),
		CORSOrigin: cfg.CORSOrigin,
		Ready:      &ready,
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
