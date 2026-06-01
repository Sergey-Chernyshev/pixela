package auth

import "context"

// UserPrincipal is the authenticated dashboard identity established by a session cookie. It is distinct
// from Principal (the ingestion API-key identity): a dashboard request is scoped to a user and their
// memberships, never to a single project. Project access is enforced per-resource at the query level.
type UserPrincipal struct {
	UserID string
	Email  string
}

// userPrincipalKeyType is the (private) context key type for the dashboard user principal.
type userPrincipalKeyType struct{}

// UserPrincipalKey is the context key under which the HTTP session middleware stores the user principal
// (via huma.WithValue); handlers read it back with UserPrincipalFromContext.
var UserPrincipalKey userPrincipalKeyType

// WithUserPrincipal returns a context carrying the authenticated dashboard user.
func WithUserPrincipal(ctx context.Context, p UserPrincipal) context.Context {
	return context.WithValue(ctx, UserPrincipalKey, p)
}

// UserPrincipalFromContext extracts the authenticated dashboard user, if any.
func UserPrincipalFromContext(ctx context.Context) (UserPrincipal, bool) {
	p, ok := ctx.Value(UserPrincipalKey).(UserPrincipal)
	return p, ok
}
