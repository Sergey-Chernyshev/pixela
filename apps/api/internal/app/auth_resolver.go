package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/httpapi"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/session"
)

// newKeyResolver builds the ingestion API-key resolver: HMAC the presented key with the server pepper,
// look it up, and (best-effort) record last-used for auditing. An unknown key is core.ErrUnauthorized.
func newKeyResolver(database *db.DB, pepper string, log *slog.Logger) httpapi.APIKeyResolver {
	return func(ctx context.Context, raw string) (auth.Principal, error) {
		row, err := database.Queries().GetAPIKeyByHash(ctx, auth.HashKey(pepper, raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.Principal{}, fmt.Errorf("unknown api key: %w", core.ErrUnauthorized)
		}
		if err != nil {
			return auth.Principal{}, fmt.Errorf("lookup api key: %w", err)
		}
		// Audit only; never fail the request on this, but log so a persistent failure is visible.
		if err := database.Queries().TouchAPIKey(ctx, row.ID); err != nil {
			log.WarnContext(ctx, "failed to record api key last-used", "key_id", row.ID, "error", err.Error())
		}
		return auth.Principal{ProjectID: row.ProjectID, KeyID: row.ID}, nil
	}
}

// newSessionResolver builds the dashboard session resolver: resolve the cookie's session id in Redis to
// a user id (sliding the TTL), then load the user. An unknown/expired session or a deleted user is
// core.ErrUnauthorized (the HTTP edge maps it to 401) — never a 500.
func newSessionResolver(database *db.DB, sessions *session.Store) httpapi.SessionResolver {
	return func(ctx context.Context, sid string) (auth.UserPrincipal, error) {
		userID, err := sessions.Resolve(ctx, sid)
		if errors.Is(err, session.ErrNotFound) {
			return auth.UserPrincipal{}, fmt.Errorf("session: %w", core.ErrUnauthorized)
		}
		if err != nil {
			return auth.UserPrincipal{}, fmt.Errorf("resolve session: %w", err)
		}
		user, err := database.Queries().GetUserByID(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserPrincipal{}, fmt.Errorf("session user gone: %w", core.ErrUnauthorized)
		}
		if err != nil {
			return auth.UserPrincipal{}, fmt.Errorf("load session user: %w", err)
		}
		return auth.UserPrincipal{UserID: user.ID, Email: user.Email}, nil
	}
}
