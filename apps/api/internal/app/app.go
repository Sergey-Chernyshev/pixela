// Package app is the composition root: it parses the subcommand, builds the logger and config,
// hand-wires dependencies, and runs the chosen mode. No DI framework — wiring is explicit and lives
// here (docs/architecture/go-backend.md §3.1). Subcommands: serve | worker | migrate | openapi.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
)

// Run dispatches the subcommand. args excludes the program name. The ctx is cancelled on SIGINT/SIGTERM.
func Run(ctx context.Context, args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	// openapi prints ONLY the spec to stdout (piped to a file) and needs no config/logger.
	if cmd == "openapi" {
		return runOpenAPI(os.Stdout)
	}

	// Reject unknown subcommands before touching the environment (clear error without requiring config).
	switch cmd {
	case "serve", "worker", "migrate", "project", "apikey", "user", "member", "seed-demo":
	default:
		return fmt.Errorf("unknown subcommand %q (want serve|worker|migrate|openapi|project|apikey|user|member|seed-demo)", cmd)
	}

	cfg, err := config.Load()
	if err != nil {
		// Returned with context; main (the top frame) logs it once — no double-handling.
		return fmt.Errorf("config: %w", err)
	}
	log := newLogger(cfg)
	slog.SetDefault(log)

	switch cmd {
	case "serve":
		return runServe(ctx, cfg, log)
	case "worker":
		return runWorker(ctx, cfg, log)
	case "project":
		return runProject(ctx, cfg, log, args[1:])
	case "apikey":
		return runAPIKey(ctx, cfg, log, args[1:])
	case "user":
		return runUser(ctx, cfg, log, args[1:])
	case "member":
		return runMember(ctx, cfg, log, args[1:])
	case "seed-demo":
		return runSeedDemo(ctx, cfg, log)
	default: // migrate (only remaining valid case)
		return runMigrate(ctx, cfg, log)
	}
}

// newLogger builds the process logger: JSON in production, human-readable text in dev, with secret
// redaction at the handler boundary (belt-and-suspenders alongside the config.Secret type).
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel), ReplaceAttr: redactSecrets}
	var h slog.Handler
	if cfg.IsProduction() {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// redactSecrets masks attributes whose key looks sensitive, so a stray log call can't leak a secret.
func redactSecrets(_ []string, a slog.Attr) slog.Attr {
	switch strings.ToLower(a.Key) {
	case "password", "token", "secret", "authorization", "dsn", "database_url", "redis_url", "session_secret":
		return slog.String(a.Key, "[REDACTED]")
	default:
		return a
	}
}
