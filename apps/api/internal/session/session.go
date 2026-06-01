// Package session implements server-side dashboard sessions backed by Redis (spec §10: server-side
// sessions are simpler to revoke than JWTs for a small self-hosted team). A session is an opaque,
// high-entropy random id (256 bits) mapping to a userId under key "sess:<id>" with a sliding TTL.
// The id is the cookie value — never a signed token carrying claims — so logout/ban is a single DEL and
// a stolen DB reveals nothing useful without Redis. HTTP concerns (cookie attributes) live in httpapi.
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// CookieName is the session cookie name set on the dashboard origin.
const CookieName = "pixela_session"

// DefaultTTL is the session lifetime; it slides forward on each successful resolve (Refresh below).
const DefaultTTL = 7 * 24 * time.Hour

// keyPrefix namespaces session keys in Redis so they never collide with other usage.
const keyPrefix = "sess:"

// ErrNotFound means the session id is unknown or expired (treated as unauthenticated by the caller).
var ErrNotFound = errors.New("session not found")

// Store creates, resolves and destroys sessions in Redis.
type Store struct {
	rdb *goredis.Client
	ttl time.Duration
}

// NewStore wires the session store over a go-redis client. A non-positive ttl falls back to DefaultTTL.
func NewStore(rdb *goredis.Client, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{rdb: rdb, ttl: ttl}
}

// TTL is the configured session lifetime (used by the HTTP layer to set the cookie Max-Age).
func (s *Store) TTL() time.Duration { return s.ttl }

// Create mints a new session for userID and returns its opaque id. The id is 32 random bytes
// (256-bit) base64url-encoded — unguessable and safe in a cookie/URL.
func (s *Store) Create(ctx context.Context, userID string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, keyPrefix+id, userID, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return id, nil
}

// Resolve returns the userID bound to a session id and slides its TTL forward (so an active user is not
// logged out mid-session). An unknown/expired id yields ErrNotFound.
func (s *Store) Resolve(ctx context.Context, id string) (string, error) {
	if id == "" {
		return "", ErrNotFound
	}
	userID, err := s.rdb.GetEx(ctx, keyPrefix+id, s.ttl).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve session: %w", err)
	}
	return userID, nil
}

// Destroy deletes a session (logout). Deleting a missing key is a no-op, not an error.
func (s *Store) Destroy(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if err := s.rdb.Del(ctx, keyPrefix+id).Err(); err != nil {
		return fmt.Errorf("destroy session: %w", err)
	}
	return nil
}

func newID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
