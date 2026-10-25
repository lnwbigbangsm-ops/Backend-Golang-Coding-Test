// Package domain holds the core business entities and rules. It has no
// dependencies on frameworks, drivers or transport layers.
package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxNameLength     = 100
	MinPasswordLength = 8
	// MaxPasswordBytes is the bcrypt input limit; longer passwords would be
	// silently truncated, so they are rejected instead.
	MaxPasswordBytes = 72
)

// User is the core user entity. PasswordHash is never exposed by adapters.
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	// CreatedBy is the ID of the user who created this account through
	// POST /users, or "" for self-registered accounts. That creator may also
	// update and delete the account.
	CreatedBy string
}

// UserUpdate describes a partial update; nil fields are left unchanged.
type UserUpdate struct {
	Name  *string
	Email *string
}

// NormalizeName trims surrounding whitespace.
func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

// NormalizeEmail trims whitespace and lower-cases the address so uniqueness
// is case-insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateName checks a normalized name.
func ValidateName(name string) string {
	switch {
	case name == "":
		return "is required"
	case utf8.RuneCountInString(name) > MaxNameLength:
		return "must be at most 100 characters"
	}
	return ""
}

// ValidateEmail checks a normalized email address. It accepts only a bare
// address (no display name such as "Bob <bob@example.com>").
func ValidateEmail(email string) string {
	if email == "" {
		return "is required"
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "must be a valid email address"
	}
	return ""
}

// ValidatePassword checks a raw (unhashed) password.
func ValidatePassword(password string) string {
	switch {
	case password == "":
		return "is required"
	case utf8.RuneCountInString(password) < MinPasswordLength:
		return "must be at least 8 characters"
	case len(password) > MaxPasswordBytes:
		return "must be at most 72 bytes"
	}
	return ""
}
