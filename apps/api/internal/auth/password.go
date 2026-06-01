package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Dashboard user passwords use argon2id (spec §10): a memory-hard KDF resistant to GPU/ASIC cracking.
// Stored as the PHC string format ("$argon2id$v=19$m=...,t=...,p=...$salt$hash"), so parameters travel
// with the hash and can evolve without a schema change. These defaults follow OWASP's argon2id guidance
// (19 MiB, 2 iterations, parallelism 1) — a sensible balance for a self-hosted dashboard login.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB → 19 MiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// errInvalidHash signals a stored hash that is not in the expected PHC format (corruption / wrong
// algorithm). Verify treats it as a non-match, never a panic.
var errInvalidHash = errors.New("invalid password hash format")

// HashPassword derives an argon2id PHC-encoded hash from a plaintext password with a fresh random salt.
func HashPassword(plaintext string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether plaintext matches an argon2id PHC hash. It recomputes the hash with the
// parameters embedded in encoded and compares in constant time. A malformed hash returns false, not an
// error to the caller path, so a corrupt row degrades to "wrong password" rather than a 500 — but the
// parse error is returned so callers can log it.
func VerifyPassword(encoded, plaintext string) (bool, error) {
	memory, time, threads, salt, want, err := decodeArgon2(encoded)
	if err != nil {
		return false, err
	}
	//nolint:gosec // len(want) is a non-negative hash length (argonKeyLen=32), never overflows uint32
	got := argon2.IDKey([]byte(plaintext), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func decodeArgon2(encoded string) (memory, time uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// "" / "argon2id" / "v=19" / "m=..,t=..,p=.." / salt / key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, errInvalidHash
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, errInvalidHash
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return 0, 0, 0, nil, nil, errInvalidHash
	}
	b64 := base64.RawStdEncoding
	if salt, err = b64.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, nil, nil, errInvalidHash
	}
	if key, err = b64.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, nil, nil, errInvalidHash
	}
	return memory, time, threads, salt, key, nil
}
