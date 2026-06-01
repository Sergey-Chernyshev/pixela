// Package auth handles ingestion API-key credentials: generation, keyed hashing for storage/lookup,
// and the authenticated principal carried in the request context. Keys are high-entropy, so a fast
// keyed hash (HMAC-SHA256 with a server pepper) is the right primitive — bcrypt is for user passwords
// (spec §10, rulebook §7.3). The raw key is shown once at creation and never stored.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// keyPrefix tags Pixela ingestion keys so they're recognizable (and greppable) in CI configs/logs.
const keyPrefix = "pxl_"

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateKey returns a fresh raw ingestion key, e.g. "pxl_4f9Qa...". 32 bytes of CSPRNG entropy
// encoded base62. Show it to the user once; persist only HashKey(pepper, raw).
func GenerateKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return keyPrefix + encodeBase62(buf), nil
}

// HashKey computes the deterministic lookup hash for a raw key: HMAC-SHA256 keyed by the server pepper,
// hex-encoded. Deterministic so the guard can look the key up by hash; keyed so a leaked DB alone
// can't reverse keys.
func HashKey(pepper, raw string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func encodeBase62(b []byte) string {
	n := new(big.Int).SetBytes(b)
	base := big.NewInt(62)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var out []byte
	for n.Cmp(zero) > 0 {
		n.DivMod(n, base, mod)
		out = append(out, base62Alphabet[mod.Int64()])
	}
	// reverse
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) == 0 {
		return "0"
	}
	return string(out)
}

// Principal is the authenticated ingestion identity: the project the API key belongs to (writes are
// confined to it — project isolation, invariant #5) and the key's id (for auditing).
type Principal struct {
	ProjectID string
	KeyID     string
}

// PrincipalKeyType is the (exported) context key under which the principal is stored, so the HTTP
// middleware can set it on a Huma context via huma.WithValue and handlers read it back here.
type PrincipalKeyType struct{}

// PrincipalKey is the context key for the authenticated principal.
var PrincipalKey PrincipalKeyType

// WithPrincipal returns a context carrying the authenticated principal.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, PrincipalKey, p)
}

// PrincipalFromContext extracts the authenticated principal, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(PrincipalKey).(Principal)
	return p, ok
}
