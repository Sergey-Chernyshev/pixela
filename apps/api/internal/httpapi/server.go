// Package httpapi builds the HTTP surface: a chi router carrying infra middleware with a Huma v2 API
// mounted under /api (the source of truth for the OpenAPI 3.1 spec). Health probes live at the root,
// outside /api and unauthenticated. See docs/architecture/go-backend.md §7, §11.3.
package httpapi

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/ingestion"
)

// Deps are the explicit dependencies of the HTTP server (constructor injection, no globals).
type Deps struct {
	Logger     *slog.Logger
	Checkers   []core.HealthChecker // probed by GET /readyz
	CORSOrigin string               // allowed dashboard origin
	Ready      *atomic.Bool         // flipped true once migrations + connections are up

	// Ingestion endpoints (nil when only emitting the OpenAPI spec). KeyResolver authenticates the
	// ingestion API key; both are wired by app in serve mode.
	Ingestion   *ingestion.Service
	KeyResolver APIKeyResolver
}

// Server wires the router and the Huma API. Build it with NewServer and serve Handler().
type Server struct {
	router chi.Router
	api    huma.API
	deps   Deps
}

// NewServer constructs the HTTP handler (Mat Ryer style: one constructor takes all deps and returns a
// configured handler; routes are listed in one place via addRoutes).
func NewServer(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	r := chi.NewRouter()

	// Infra middleware (connection-level), before routing.
	r.Use(chimw.RequestID)
	// NB: chi's RealIP is deprecated (X-Forwarded-For spoofing). Real client IP behind Traefik is
	// derived in a later phase from a trusted proxy header allowlist, not here.
	r.Use(recoverer(deps.Logger))
	r.Use(accessLog(deps.Logger))
	if deps.CORSOrigin != "" {
		r.Use(cors(deps.CORSOrigin))
	}

	// Route Huma's built-in errors through the contract's { error: { code, message } } envelope.
	installErrorEnvelope()

	// Huma API under /api — the OpenAPI 3.1 source of truth.
	cfg := huma.DefaultConfig("Pixela API", "0.1.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		apiKeySchemeName: {
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "Ingestion credential. Send `Authorization: ApiKey <key>`.",
		},
	}
	// Mount the Huma API under /api so operation paths (/v1/...) resolve to /api/v1/... — the server
	// URL above is documentation only and does not affect routing.
	apiMux := chi.NewMux()
	api := humachi.New(apiMux, cfg)

	// Enforce the API key on operations that declare it (the guard is a no-op for emit-only servers
	// where KeyResolver is nil, since no requests are served).
	if deps.KeyResolver != nil {
		api.UseMiddleware(apiKeyMiddleware(api, deps.KeyResolver))
	}
	registerIngestion(api, deps.Ingestion, deps.Logger)
	r.Mount("/api", apiMux)

	s := &Server{router: r, api: api, deps: deps}
	s.addRoutes()
	return s
}

// addRoutes lists the entire non-Huma route surface in one place.
func (s *Server) addRoutes() {
	s.router.Get("/healthz", handleLiveness())
	s.router.Get("/readyz", handleReadiness(s.deps))
}

// Handler returns the composed http.Handler.
func (s *Server) Handler() http.Handler { return s.router }

// API exposes the Huma API (for registering operations in later phases).
func (s *Server) API() huma.API { return s.api }

// OpenAPIYAML renders the OpenAPI 3.1 document without booting the server (used by `pixela openapi`).
func (s *Server) OpenAPIYAML() ([]byte, error) {
	return s.api.OpenAPI().YAML()
}
