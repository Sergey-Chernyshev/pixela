package core

import "errors"

// Domain sentinel errors. Always return them WRAPPED (fmt.Errorf("...: %w", ErrX)); callers use
// errors.Is, never ==. See docs/architecture/go-backend.md §5. The HTTP edge maps these to the
// { "error": { "code", "message" } } envelope via ErrorCode below.
var (
	ErrNotFound         = errors.New("not found")
	ErrBuildNotFound    = errors.New("build not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbiddenProject = errors.New("forbidden: project not accessible")
	ErrValidation       = errors.New("validation failed")
	ErrHashMismatch     = errors.New("snapshot image hash mismatch")
	ErrImageTooLarge    = errors.New("image too large")
	ErrBuildFinalized   = errors.New("build already finalized")
	ErrConflict         = errors.New("conflict")
	// ErrInvalidCredentials is a failed dashboard login (wrong email or password). It maps to 401 with a
	// deliberately generic message so the response never reveals whether the email exists.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// ErrorCode is the stable, machine-readable code returned to API clients (mirrors the spec's
// error envelope). It is intentionally a closed set.
type ErrorCode string

const (
	CodeValidation       ErrorCode = "VALIDATION_ERROR"
	CodeUnauthorized     ErrorCode = "UNAUTHORIZED"
	CodeForbiddenProject ErrorCode = "FORBIDDEN_PROJECT"
	CodeNotFound         ErrorCode = "NOT_FOUND"
	CodeBuildNotFound    ErrorCode = "BUILD_NOT_FOUND"
	CodeHashMismatch     ErrorCode = "SNAPSHOT_HASH_MISMATCH"
	CodeImageTooLarge    ErrorCode = "IMAGE_TOO_LARGE"
	CodeBuildFinalized   ErrorCode = "BUILD_ALREADY_FINALIZED"
	CodeInvalidCreds     ErrorCode = "INVALID_CREDENTIALS" //nolint:gosec // error code identifier, not a credential
	CodeConflict         ErrorCode = "CONFLICT"
	CodeInternal         ErrorCode = "INTERNAL"
)
