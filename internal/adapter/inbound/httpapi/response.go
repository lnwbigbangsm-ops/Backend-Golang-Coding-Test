package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
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
	var berr *bodyError
	switch {
	case errors.As(err, &berr):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: errorDetail{
			Code: "bad_request", Message: berr.msg, Fields: berr.fields,
		}})
	case errors.As(err, &verr):
		// 422: the body was well-formed JSON but its values are not acceptable.
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: errorDetail{
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
	case errors.Is(err, domain.ErrForbidden):
		writeErrorCode(w, http.StatusForbidden, "forbidden", "you can only modify your own account")
	default:
		logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// bodyError is a malformed request body. Its messages are our own, not the
// encoding/json error text, so clients never come to depend on Go's wording.
type bodyError struct {
	msg    string
	fields map[string]string
}

func (e *bodyError) Error() string { return e.msg }

// decodeJSON reads a single JSON object, rejecting unknown fields, trailing
// data and bodies larger than maxBodyBytes.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return toBodyError(err)
	}
	if dec.More() {
		return &bodyError{msg: "request body must contain a single JSON object"}
	}
	return nil
}

func toBodyError(err error) *bodyError {
	var maxErr *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return &bodyError{msg: "request body is empty"}
	case errors.As(err, &maxErr):
		return &bodyError{msg: fmt.Sprintf("request body must not exceed %d bytes", maxBodyBytes)}
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return &bodyError{msg: "request body must be a JSON object"}
		}
		return &bodyError{msg: "request body has a field of the wrong type", fields: map[string]string{typeErr.Field: "has the wrong type"}}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no typed error for unknown fields.
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return &bodyError{msg: "request body contains an unknown field", fields: map[string]string{field: "is not allowed"}}
	default:
		return &bodyError{msg: "request body is not valid JSON"}
	}
}
