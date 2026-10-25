package domain

import (
	"errors"
	"sort"
	"strings"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	// ErrForbidden means the caller is authenticated but may not act on the
	// target resource (for example, modifying another user's account).
	ErrForbidden = errors.New("forbidden")
)

// ValidationError collects per-field validation messages.
type ValidationError struct {
	Fields map[string]string
}

// Add records a message for field when msg is non-empty.
func (e *ValidationError) Add(field, msg string) {
	if msg == "" {
		return
	}
	if e.Fields == nil {
		e.Fields = make(map[string]string)
	}
	e.Fields[field] = msg
}

// OrNil returns e as an error if it holds any messages, otherwise nil.
func (e *ValidationError) OrNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " " + e.Fields[k]
	}
	return "validation failed: " + strings.Join(parts, "; ")
}
