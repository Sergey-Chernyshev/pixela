package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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

// runUser is the admin bootstrap: `pixela user create <email> <password> [name]`. It stores only the
// argon2id hash of the password, never the plaintext. Dashboard self-service signup is out of scope for
// a small self-hosted team — the operator seeds users.
func runUser(ctx context.Context, cfg config.Config, log *slog.Logger, sub []string) error {
	if len(sub) < 3 || sub[0] != "create" {
		return errors.New("usage: pixela user create <email> <password> [name]")
	}
	email, password := sub[1], sub[2]
	var name *string
	if len(sub) >= 4 && sub[3] != "" {
		n := sub[3]
		name = &n
	}

	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user, err := database.Queries().CreateUser(ctx, db.CreateUserParams{
		ID: core.NewID(), Email: email, Name: name, PasswordHash: &hash,
	})
	if err != nil {
		return fmt.Errorf("create user (email %q may already exist): %w", email, err)
	}
	fmt.Printf("created user %q (id %s)\n", user.Email, user.ID)
	return nil
}

// runMember is the admin bootstrap: `pixela member add <user-email> <project-slug> [role]`. Grants a
// user access to a project (role OWNER|MEMBER, default MEMBER). Idempotent: re-running updates the role
// (the query upserts on the unique (user_id, project_id) pair).
func runMember(ctx context.Context, cfg config.Config, log *slog.Logger, sub []string) error {
	if len(sub) < 3 || sub[0] != "add" {
		return errors.New("usage: pixela member add <user-email> <project-slug> [role: OWNER|MEMBER]")
	}
	email, slug := sub[1], sub[2]
	role := db.RoleMEMBER
	if len(sub) >= 4 {
		role = db.Role(strings.ToUpper(sub[3]))
		if role != db.RoleOWNER && role != db.RoleMEMBER {
			return fmt.Errorf("invalid role %q (want OWNER or MEMBER)", sub[3])
		}
	}

	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	user, err := database.Queries().GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user %q not found — create it first: pixela user create %s <password>", email, email)
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	project, err := database.Queries().GetProjectBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("project %q not found — create it first: pixela project create <name> %s", slug, slug)
	}
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	m, err := database.Queries().CreateMembership(ctx, db.CreateMembershipParams{
		ID: core.NewID(), UserID: user.ID, ProjectID: project.ID, Role: role,
	})
	if err != nil {
		return fmt.Errorf("create membership: %w", err)
	}
	fmt.Printf("added %q to project %q as %s\n", email, slug, m.Role)
	return nil
}
