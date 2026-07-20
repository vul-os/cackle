package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// apiError is the wire shape of every error Cackle's HTTP API returns:
//
//	{"error":{"code":"...","message":"..."}}
//
// message is meant for a human; code is meant for programmatic handling.
// Internal errors (DB failures, panics, provider transport errors) are
// NEVER echoed here — see internalError.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

// Error codes used across handlers. Treat any code not in this list as a
// generic failure rather than special-casing on an incomplete set (see
// docs/API.md).
const (
	codeInvalidRequest  = "invalid_request"
	codeUnauthorized    = "unauthorized"
	codeForbidden       = "forbidden"
	codeNotFound        = "not_found"
	codeConflict        = "conflict"
	codeRateLimited     = "rate_limited"
	codeInternal        = "internal_error"
	codeFrontendMissing = "frontend_not_built"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, codeInvalidRequest, msg)
}

func unauthorized(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusUnauthorized, codeUnauthorized, msg)
}

func forbidden(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusForbidden, codeForbidden, msg)
}

func notFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, codeNotFound, msg)
}

func conflict(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusConflict, codeConflict, msg)
}

func tooManyRequests(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusTooManyRequests, codeRateLimited, msg)
}

// internalError logs the real error server-side (never to the client) and
// returns a flat, non-leaking 500. Never pass err.Error() to writeError
// directly for anything that touched the database, a provider, or an
// internal invariant — this is the one place that boundary is enforced.
func internalError(w http.ResponseWriter, logger *slog.Logger, context string, err error) {
	if logger != nil {
		logger.Error("internal error", "context", context, "error", err)
	}
	writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}
