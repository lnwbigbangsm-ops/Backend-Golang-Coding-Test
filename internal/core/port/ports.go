// Package port defines the interfaces between the application core and the
// outside world. Driven (outbound) ports are implemented by infrastructure
// adapters; driving (inbound) ports are implemented by the core services and
// consumed by transport adapters (HTTP, gRPC, scheduler).
package port

import (
	"context"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
)

// ---- Driven ports ----

// UserRepository persists users. Implementations must return
// domain.ErrUserNotFound and domain.ErrEmailAlreadyExists where applicable.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	// List returns up to limit users ordered by creation time (oldest first),
	// skipping the first offset.
	List(ctx context.Context, offset, limit int) ([]*domain.User, error)
	Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

// IdempotencyRecord is a stored attempt for one (scope, key) pair.
type IdempotencyRecord struct {
	Scope       string
	Key         string
	RequestHash string
	ClaimedAt   time.Time
	Completed   bool
	StatusCode  int
	Location    string
	Body        []byte
}

// IdempotencyStore records Idempotency-Key usage. Claim must be atomic: of
// any number of concurrent claims for the same scope and key, exactly one
// returns claimed=true.
type IdempotencyStore interface {
	// Claim inserts rec as in progress. If a record already exists it is
	// returned with claimed=false, unless it is an in-progress record with the
	// same request hash claimed before staleBefore, in which case it is taken
	// over and claimed=true.
	Claim(ctx context.Context, rec IdempotencyRecord, staleBefore time.Time) (existing *IdempotencyRecord, claimed bool, err error)
	// Complete stores the response of a claimed request.
	Complete(ctx context.Context, scope, key string, status int, location string, body []byte) error
	// Release deletes a claim so the request can be retried.
	Release(ctx context.Context, scope, key string) error
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

// UserPage is one page of users plus the total across all pages.
type UserPage struct {
	Users      []*domain.User
	Page       domain.Page
	TotalItems int64
}

// UserService is the use-case API for managing users. actorID is the
// authenticated caller; users may only modify their own account.
type UserService interface {
	Create(ctx context.Context, in CreateUserInput) (*domain.User, error)
	Get(ctx context.Context, id string) (*domain.User, error)
	List(ctx context.Context, page domain.Page) (*UserPage, error)
	Update(ctx context.Context, actorID, id string, update domain.UserUpdate) (*domain.User, error)
	Delete(ctx context.Context, actorID, id string) error
	Count(ctx context.Context) (int64, error)
}

// AuthService is the use-case API for registration and authentication.
type AuthService interface {
	Register(ctx context.Context, in CreateUserInput) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*LoginResult, error)
	Authenticate(ctx context.Context, token string) (*TokenClaims, error)
}
