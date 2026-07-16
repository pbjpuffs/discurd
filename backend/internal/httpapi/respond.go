// Package httpapi implements the REST handlers, middleware, and router for
// the api service (docs/ARCHITECTURE.md §7).
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"discurd/internal/store"
)

// Machine error codes (contract §7).
const (
	CodeValidationFailed   = "validation_failed"
	CodeInvalidCredentials = "invalid_credentials"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeRateLimited        = "rate_limited"
	CodeTooLarge           = "too_large"
	CodeInternal           = "internal"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeStoreError maps repository sentinel errors onto the contract error
// shape; anything unexpected becomes a logged 500.
func writeStoreError(w http.ResponseWriter, logger *slog.Logger, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, CodeNotFound, notFoundMsg)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, CodeConflict, err.Error())
	default:
		logger.Error("store error", "error", err.Error())
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
	}
}

// decodeJSON parses a request body into v, rejecting unknown-shaped garbage.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "invalid JSON body")
		return false
	}
	return true
}
