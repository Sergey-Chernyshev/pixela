// Command pixela is the single Pixela backend binary. Subcommands: serve | worker | migrate | openapi.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/app"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run holds the deferred signal cleanup so main can os.Exit without skipping it.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, os.Args[1:])
}
