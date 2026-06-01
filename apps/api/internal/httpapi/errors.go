package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

// apiError is the wire shape of every error: { "error": { "code", "message" } } (API contract §Errors).
// It implements huma.StatusError, so returning it from a handler makes Huma write exactly this body
// with the chosen status — and overriding huma.NewError routes Huma's own validation errors here too.
type apiError struct {
	status int          // unexported: drives the HTTP status, not serialized
	Detail apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    core.ErrorCode `json:"code"`
	Message string         `json:"message"`
}

func (e *apiError) Error() string  { return e.Detail.Message }
func (e *apiError) GetStatus() int { return e.status }

func newAPIError(status int, code core.ErrorCode, message string) *apiError {
	return &apiError{status: status, Detail: apiErrorBody{Code: code, Message: message}}
}

// mapError converts a domain error into the wire error, logging unexpected (5xx) causes in full while
// returning a non-leaky message to the client (rulebook §5: one place maps domain → HTTP).
func mapError(log *slog.Logger, err error) *apiError {
	switch {
	case errors.Is(err, core.ErrUnauthorized):
		return newAPIError(http.StatusUnauthorized, core.CodeUnauthorized, "Authentication required")
	case errors.Is(err, core.ErrInvalidCredentials):
		return newAPIError(http.StatusUnauthorized, core.CodeInvalidCreds, "Invalid email or password")
	case errors.Is(err, core.ErrForbiddenProject):
		return newAPIError(http.StatusForbidden, core.CodeForbiddenProject, "Resource not in your project")
	case errors.Is(err, core.ErrBuildNotFound):
		return newAPIError(http.StatusNotFound, core.CodeBuildNotFound, "Build not found")
	case errors.Is(err, core.ErrNotFound):
		return newAPIError(http.StatusNotFound, core.CodeNotFound, "Resource not found")
	case errors.Is(err, core.ErrBuildFinalized):
		return newAPIError(http.StatusConflict, core.CodeBuildFinalized, "Build is already finalized")
	case errors.Is(err, core.ErrConflict):
		return newAPIError(http.StatusConflict, core.CodeConflict, "Snapshot is not in a reviewable state")
	case errors.Is(err, core.ErrHashMismatch):
		return newAPIError(http.StatusBadRequest, core.CodeHashMismatch, "Uploaded bytes do not match the declared sha256")
	case errors.Is(err, core.ErrImageTooLarge):
		return newAPIError(http.StatusBadRequest, core.CodeImageTooLarge, "Image exceeds the maximum allowed size")
	case errors.Is(err, core.ErrValidation):
		return newAPIError(http.StatusBadRequest, core.CodeValidation, "Request validation failed")
	default:
		log.Error("unhandled ingestion error", "error", err.Error())
		return newAPIError(http.StatusInternalServerError, core.CodeInternal, "Internal server error")
	}
}

// installErrorEnvelope routes Huma's built-in errors (request validation, auth failures it generates)
// through the same { error: { code, message } } envelope. Called once from NewServer.
func installErrorEnvelope() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		// Huma emits 422 for request-body validation; the contract uses 400 VALIDATION_ERROR.
		code := core.CodeInternal
		switch {
		case status == http.StatusUnauthorized:
			code = core.CodeUnauthorized
		case status == http.StatusForbidden:
			code = core.CodeForbiddenProject
		case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
			status, code = http.StatusBadRequest, core.CodeValidation
		case status == http.StatusNotFound:
			code = core.CodeNotFound
		case status < http.StatusInternalServerError:
			code = core.CodeValidation
		}
		// Never echo an internal cause to clients on 5xx — return a generic message (the cause is
		// logged at the handler edge / by Huma). Field-level validation detail is safe on 4xx only.
		if status >= http.StatusInternalServerError {
			return newAPIError(status, core.CodeInternal, "Internal server error")
		}
		details := make([]string, 0, len(errs))
		for _, e := range errs {
			if e != nil {
				details = append(details, e.Error())
			}
		}
		if len(details) > 0 {
			msg = msg + ": " + strings.Join(details, "; ")
		}
		return newAPIError(status, code, msg)
	}
}
