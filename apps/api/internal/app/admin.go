package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
)

// runProject is the admin bootstrap: `pixela project create <name> <slug>`. Dashboard project CRUD
// with session auth arrives in Phase 4; this lets a self-hoster (and CI) get going now.
func runProject(ctx context.Context, cfg config.Config, log *slog.Logger, sub []string) error {
	if len(sub) < 3 || sub[0] != "create" {
		return errors.New("usage: pixela project create <name> <slug>")
	}
	name, slug := sub[1], sub[2]

	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	project, err := database.Queries().CreateProject(ctx, db.CreateProjectParams{
		ID:            core.NewID(),
		Name:          name,
		Slug:          slug,
		DefaultBranch: "main",
	})
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	fmt.Printf("created project %q (id %s)\n", project.Slug, project.ID)
	return nil
}

// runAPIKey is the admin bootstrap: `pixela apikey create <project-slug> [key-name]`. It mints a key,
// stores only its HMAC hash, and prints the raw key ONCE.
func runAPIKey(ctx context.Context, cfg config.Config, log *slog.Logger, sub []string) error {
	if len(sub) < 2 || sub[0] != "create" {
		return errors.New("usage: pixela apikey create <project-slug> [key-name]")
	}
	slug := sub[1]
	keyName := "default"
	if len(sub) >= 3 {
		keyName = sub[2]
	}

	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	project, err := database.Queries().GetProjectBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("project %q not found — create it first: pixela project create <name> %s", slug, slug)
	}
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	raw, err := auth.GenerateKey()
	if err != nil {
		return err
	}
	if _, err := database.Queries().CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        core.NewID(),
		ProjectID: project.ID,
		KeyHash:   auth.HashKey(cfg.SessionSecret.Reveal(), raw),
		Name:      keyName,
	}); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	fmt.Printf("API key for project %q (shown ONCE — store it now):\n\n  %s\n\nUse: Authorization: ApiKey %s\n", slug, raw, raw)
	return nil
}
