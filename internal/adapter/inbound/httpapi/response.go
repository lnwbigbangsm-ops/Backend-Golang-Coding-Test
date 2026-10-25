package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"backend-golang-coding-test/internal/core/domain"
)

const maxBodyBytes = 1 << 20 // 1 MiB

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErrorCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: msg}})
}

// writeError maps domain errors to HTTP responses. Unexpected errors are
// logged and hidden behind a generic 500.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var verr *domain.ValidationError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: errorDetail{
			Code: "validation_error", Message: "request validation failed", Fields: verr.Fields,
		}})
	case errors.Is(err, domain.ErrUserNotFound):
		writeErrorCode(w, http.StatusNotFound, "not_found", "user not found")
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		writeErrorCode(w, http.StatusConflict, "email_exists", "email already exists")
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeErrorCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, domain.ErrInvalidToken):
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeErrorCode(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
	default:
		logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// decodeJSON reads a single JSON object, rejecting unknown fields, trailing
// data and bodies larger than maxBodyBytes.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		case errors.As(err, &maxErr):
			return fmt.Errorf("request body must not exceed %d bytes", maxBodyBytes)
		default:
			return fmt.Errorf("invalid JSON body: %v", err)
		}
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
