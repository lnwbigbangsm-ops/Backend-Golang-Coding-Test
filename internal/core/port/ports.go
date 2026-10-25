// Package port defines the interfaces between the application core and the
// outside world. Driven (outbound) ports are implemented by infrastructure
// adapters; driving (inbound) ports are implemented by the core services and
// consumed by transport adapters (HTTP, gRPC, scheduler).
package port

import (
	"context"
	"time"

	"backend-golang-coding-test/internal/core/domain"
)

// ---- Driven ports ----

// UserRepository persists users. Implementations must return
// domain.ErrUserNotFound and domain.ErrEmailAlreadyExists where applicable.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	List(ctx context.Context) ([]*domain.User, error)
	Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// TokenClaims is the identity carried in an access token.
type TokenClaims struct {
	UserID    string
	Email     string
	ExpiresAt time.Time
}

// TokenService issues and verifies access tokens.
type TokenService interface {
	Issue(user *domain.User) (token string, expiresAt time.Time, err error)
	Verify(token string) (*TokenClaims, error)
}

// ---- Driving ports ----

// CreateUserInput is the input for registering or creating a user.
type CreateUserInput struct {
	Name     string
	Email    string
	Password string
}

// LoginResult is returned after successful authentication.
type LoginResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	User        *domain.User
}

// UserService is the use-case API for managing users.
type UserService interface {
	Create(ctx context.Context, in CreateUserInput) (*domain.User, error)
	Get(ctx context.Context, id string) (*domain.User, error)
	List(ctx context.Context) ([]*domain.User, error)
	Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

// AuthService is the use-case API for registration and authentication.
type AuthService interface {
	Register(ctx context.Context, in CreateUserInput) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*LoginResult, error)
	Authenticate(ctx context.Context, token string) (*TokenClaims, error)
}
