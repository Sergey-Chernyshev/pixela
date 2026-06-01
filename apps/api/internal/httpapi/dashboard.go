package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/dashboard"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/session"
)

// sessionSchemeName is the OpenAPI security scheme for dashboard cookie sessions (a scheme id, not a
// secret).
const sessionSchemeName = "SessionCookie" //nolint:gosec // scheme name, not a credential

// sessionSecurity is the per-operation requirement that turns on the session guard.
var sessionSecurity = []map[string][]string{{sessionSchemeName: {}}}

// SessionResolver turns a session id (the cookie value) into the authenticated dashboard user. The app
// supplies it (resolve in Redis → load the user); it returns core.ErrUnauthorized for unknown/expired
// sessions.
type SessionResolver func(ctx context.Context, sessionID string) (auth.UserPrincipal, error)

// sessionIDKeyType is the private context key under which the middleware stashes the raw session id so
// logout can revoke exactly the presented session.
type sessionIDKeyType struct{}

var sessionIDKey sessionIDKeyType

// sessionMiddleware enforces dashboard auth ONLY on operations that declare the SessionCookie scheme
// (docs == enforcement, rulebook §7.3). Public ops (login) and ingestion ops pass through untouched.
//
//nolint:contextcheck // a Huma middleware carries its context via huma.Context; ctx.Context() is the request ctx
func sessionMiddleware(api huma.API, resolve SessionResolver) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if !operationRequiresSession(ctx.Operation()) {
			next(ctx)
			return
		}
		sid := cookieValue(ctx, session.CookieName)
		if sid == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Authentication required")
			return
		}
		principal, err := resolve(ctx.Context(), sid)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Authentication required")
			return
		}
		c := huma.WithValue(ctx, auth.UserPrincipalKey, principal)
		next(huma.WithValue(c, sessionIDKey, sid))
	}
}

func operationRequiresSession(op *huma.Operation) bool {
	for _, requirement := range op.Security {
		if _, ok := requirement[sessionSchemeName]; ok {
			return true
		}
	}
	return false
}

// cookieValue extracts one cookie from the request's Cookie header via the stdlib parser (no manual
// splitting — quoting and attributes are handled correctly).
func cookieValue(ctx huma.Context, name string) string {
	header := ctx.Header("Cookie")
	if header == "" {
		return ""
	}
	req := &http.Request{Header: http.Header{"Cookie": []string{header}}}
	c, err := req.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// registerDashboard wires the human-facing dashboard endpoints (API contract §"Dashboard"). Reads
// declare sessionSecurity; login is public (sets the cookie). cookieTTL/cookieSecure shape the
// Set-Cookie attributes.
func registerDashboard(api huma.API, svc *dashboard.Service, log *slog.Logger, cookieTTL time.Duration, cookieSecure bool) {
	huma.Register(api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/v1/auth/login",
		Summary: "Log in to the dashboard", Tags: []string{"dashboard"},
	}, func(ctx context.Context, in *loginInput) (*loginOutput, error) {
		principal, sid, err := svc.Login(ctx, in.Body.Email, in.Body.Password)
		if err != nil {
			return nil, mapError(log, err)
		}
		out := &loginOutput{SetCookie: sessionCookie(sid, cookieTTL, cookieSecure)}
		out.Body.UserID = principal.UserID
		out.Body.Email = principal.Email
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/v1/auth/logout",
		Summary: "Log out (revoke the session)", Tags: []string{"dashboard"},
		Security: sessionSecurity,
	}, func(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
		if err := svc.Logout(ctx, sessionIDFromContext(ctx)); err != nil {
			return nil, mapError(log, err)
		}
		out := &logoutOutput{SetCookie: clearCookie(cookieSecure)}
		out.Body.OK = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/v1/auth/me",
		Summary: "Current user", Tags: []string{"dashboard"}, Security: sessionSecurity,
	}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
		p, err := requireUser(ctx)
		if err != nil {
			return nil, err
		}
		user, err := svc.Me(ctx, p.UserID)
		if err != nil {
			return nil, mapError(log, err)
		}
		return &meOutput{Body: user}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listProjects", Method: http.MethodGet, Path: "/v1/projects",
		Summary: "Projects you can access", Tags: []string{"dashboard"}, Security: sessionSecurity,
	}, func(ctx context.Context, _ *struct{}) (*listProjectsOutput, error) {
		p, err := requireUser(ctx)
		if err != nil {
			return nil, err
		}
		projects, err := svc.ListProjects(ctx, p.UserID)
		if err != nil {
			return nil, mapError(log, err)
		}
		out := &listProjectsOutput{}
		out.Body.Projects = projects
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "listBuilds", Method: http.MethodGet, Path: "/v1/projects/{projectId}/builds",
		Summary: "Build feed for a project", Tags: []string{"dashboard"}, Security: sessionSecurity,
	}, func(ctx context.Context, in *listBuildsInput) (*listBuildsOutput, error) {
		p, err := requireUser(ctx)
		if err != nil {
			return nil, err
		}
		page, err := svc.ListBuilds(ctx, p.UserID, in.ProjectID, in.Branch, in.Status, in.Page)
		if err != nil {
			return nil, mapError(log, err)
		}
		return &listBuildsOutput{Body: page}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getBuild", Method: http.MethodGet, Path: "/v1/builds/{buildId}",
		Summary: "Build detail with snapshots", Tags: []string{"dashboard"}, Security: sessionSecurity,
	}, func(ctx context.Context, in *getBuildInput) (*getBuildOutput, error) {
		p, err := requireUser(ctx)
		if err != nil {
			return nil, err
		}
		detail, err := svc.GetBuild(ctx, p.UserID, in.BuildID)
		if err != nil {
			return nil, mapError(log, err)
		}
		return &getBuildOutput{Body: detail}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getSnapshot", Method: http.MethodGet, Path: "/v1/snapshots/{snapshotId}",
		Summary: "Snapshot review payload (presigned images + history)", Tags: []string{"dashboard"},
		Security: sessionSecurity,
	}, func(ctx context.Context, in *getSnapshotInput) (*getSnapshotOutput, error) {
		p, err := requireUser(ctx)
		if err != nil {
			return nil, err
		}
		review, err := svc.GetSnapshot(ctx, p.UserID, in.SnapshotID)
		if err != nil {
			return nil, mapError(log, err)
		}
		return &getSnapshotOutput{Body: review}, nil
	})
}

// requireUser returns the authenticated dashboard user or a 401 (the middleware guarantees it on
// secured ops; this is defense in depth).
func requireUser(ctx context.Context) (auth.UserPrincipal, error) {
	p, ok := auth.UserPrincipalFromContext(ctx)
	if !ok {
		return auth.UserPrincipal{}, newAPIError(http.StatusUnauthorized, core.CodeUnauthorized, "Authentication required")
	}
	return p, nil
}

func sessionIDFromContext(ctx context.Context) string {
	sid, _ := ctx.Value(sessionIDKey).(string)
	return sid
}

// sessionCookie builds the dashboard session cookie. HttpOnly (no JS access), SameSite=Lax (sent on
// top-level navigations, blocks CSRF on cross-site POSTs), Secure in production (TLS-only).
func sessionCookie(value string, maxAge time.Duration, secure bool) http.Cookie {
	//nolint:gosec // Secure is set from cfg.IsProduction() (TLS in prod); dev/test serve plain http. HttpOnly+SameSite are always set.
	return http.Cookie{
		Name: session.CookieName, Value: value, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(maxAge.Seconds()),
	}
}

// clearCookie expires the session cookie (logout).
func clearCookie(secure bool) http.Cookie {
	//nolint:gosec // expiry cookie (MaxAge<0); mirrors sessionCookie's Secure/HttpOnly/SameSite policy
	return http.Cookie{
		Name: session.CookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: -1,
	}
}

// ---- DTOs ----

type loginInput struct {
	Body struct {
		Email    string `json:"email" format:"email" minLength:"3" doc:"Account email"`
		Password string `json:"password" minLength:"1" doc:"Account password"`
	}
}

type loginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
	}
}

type logoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		OK bool `json:"ok"`
	}
}

type meOutput struct {
	Body dashboard.User
}

type listProjectsOutput struct {
	Body struct {
		Projects []dashboard.ProjectView `json:"projects"`
	}
}

type listBuildsInput struct {
	ProjectID string `path:"projectId"`
	Branch    string `query:"branch" doc:"Filter by branch (optional)"`
	Status    string `query:"status" doc:"Filter by build status (optional)"`
	Page      int    `query:"page" minimum:"1" maximum:"1000000" default:"1"`
}

type listBuildsOutput struct {
	Body dashboard.BuildsPage
}

type getBuildInput struct {
	BuildID string `path:"buildId"`
}

type getBuildOutput struct {
	Body dashboard.BuildDetail
}

type getSnapshotInput struct {
	SnapshotID string `path:"snapshotId"`
}

type getSnapshotOutput struct {
	Body dashboard.SnapshotReview
}
