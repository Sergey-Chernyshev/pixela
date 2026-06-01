package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
)

// apiKeySchemeName is the OpenAPI security scheme for CI ingestion keys (a scheme identifier, not a
// secret).
const apiKeySchemeName = "ApiKeyAuth" //nolint:gosec // scheme name, not a credential

// APIKeyResolver turns a raw "ApiKey <key>" credential into a principal. The app supplies it (HMAC the
// key, look it up, touch last-used); it must return core.ErrUnauthorized for an unknown/invalid key.
type APIKeyResolver func(ctx context.Context, rawKey string) (auth.Principal, error)

// apiKeyMiddleware enforces ingestion auth ONLY on operations that declare the ApiKeyAuth scheme —
// docs == enforcement (rulebook §7.3). It reads "Authorization: ApiKey <key>", resolves the principal,
// and stashes it for the handler. Operations without the scheme (health, dashboard reads) pass through.
//
//nolint:contextcheck // a Huma middleware carries its context via huma.Context; ctx.Context() is the request ctx
func apiKeyMiddleware(api huma.API, resolve APIKeyResolver) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if !operationRequiresAPIKey(ctx.Operation()) {
			next(ctx)
			return
		}
		raw, ok := parseAPIKey(ctx.Header("Authorization"))
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Authentication required")
			return
		}
		principal, err := resolve(ctx.Context(), raw)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Invalid API key")
			return
		}
		next(huma.WithValue(ctx, auth.PrincipalKey, principal))
	}
}

func operationRequiresAPIKey(op *huma.Operation) bool {
	for _, requirement := range op.Security {
		if _, ok := requirement[apiKeySchemeName]; ok {
			return true
		}
	}
	return false
}

func parseAPIKey(header string) (string, bool) {
	const prefix = "ApiKey "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	key := strings.TrimSpace(header[len(prefix):])
	return key, key != ""
}
