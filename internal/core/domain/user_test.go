package domain_test

import (
	"strings"
	"testing"

	"backend-golang-coding-test/internal/core/domain"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"alice@example.com", true},
		{"a.b+tag@sub.example.co.th", true},
		{"", false},
		{"alice", false},
		{"alice@", false},
		{"@example.com", false},
		{"alice@localhost", false},
		{"Alice <alice@example.com>", false},
		{"alice example@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := domain.ValidateEmail(tt.email) == ""
			if got != tt.valid {
				t.Errorf("ValidateEmail(%q) valid = %v, want %v", tt.email, got, tt.valid)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"ok", "Alice", true},
		{"thai", "สมชาย ใจดี", true},
		{"empty", "", false},
		{"max length", strings.Repeat("ก", domain.MaxNameLength), true},
		{"too long", strings.Repeat("a", domain.MaxNameLength+1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ValidateName(tt.input) == ""; got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"ok", "s3cretPass", true},
		{"empty", "", false},
		{"too short", "1234567", false},
		{"bcrypt limit", strings.Repeat("a", domain.MaxPasswordBytes), true},
		{"over bcrypt limit", strings.Repeat("a", domain.MaxPasswordBytes+1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ValidatePassword(tt.input) == ""; got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	if got := domain.NormalizeEmail("  Alice@Example.COM "); got != "alice@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
	if got := domain.NormalizeName("  Alice  "); got != "Alice" {
		t.Errorf("NormalizeName = %q", got)
	}
}

func TestValidationError(t *testing.T) {
	verr := &domain.ValidationError{}
	verr.Add("name", "")
	if verr.OrNil() != nil {
		t.Fatal("empty message must not be recorded")
	}
	verr.Add("name", "is required")
	verr.Add("email", "is required")
	err := verr.OrNil()
	if err == nil {
		t.Fatal("expected error")
	}
	if want := "validation failed: email is required; name is required"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
