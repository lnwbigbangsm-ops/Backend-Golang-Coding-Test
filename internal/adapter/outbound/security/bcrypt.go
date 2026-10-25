// Package security provides password hashing and JWT adapters.
package security

import (
	"golang.org/x/crypto/bcrypt"

	"backend-golang-coding-test/internal/core/port"
)

// BcryptHasher implements port.PasswordHasher with bcrypt.
type BcryptHasher struct {
	cost int
}

var _ port.PasswordHasher = (*BcryptHasher)(nil)

// NewBcryptHasher returns a hasher using the given cost, or bcrypt.DefaultCost
// when cost is out of range.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (h *BcryptHasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
