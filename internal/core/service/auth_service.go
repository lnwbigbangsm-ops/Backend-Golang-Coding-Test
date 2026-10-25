package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// AuthService implements port.AuthService.
type AuthService struct {
	users  port.UserService
	repo   port.UserRepository
	hasher port.PasswordHasher
	tokens port.TokenService
	// dummyHash is compared against when the email is unknown so that login
	// takes the same time whether or not the account exists.
	dummyHash string
}

var _ port.AuthService = (*AuthService)(nil)

// NewAuthService builds an AuthService.
func NewAuthService(users port.UserService, repo port.UserRepository, hasher port.PasswordHasher, tokens port.TokenService) (*AuthService, error) {
	dummy, err := hasher.Hash("timing-equalizer-password")
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	return &AuthService{users: users, repo: repo, hasher: hasher, tokens: tokens, dummyHash: dummy}, nil
}

// Register creates a new account.
func (s *AuthService) Register(ctx context.Context, in port.CreateUserInput) (*domain.User, error) {
	return s.users.Create(ctx, in)
}

// Login verifies credentials and issues an access token. Unknown emails and
// wrong passwords both return domain.ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, email, password string) (*port.LoginResult, error) {
	email = domain.NormalizeEmail(email)
	if email == "" || password == "" {
		verr := &domain.ValidationError{}
		if email == "" {
			verr.Add("email", "is required")
		}
		if password == "" {
			verr.Add("password", "is required")
		}
		return nil, verr
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		_ = s.hasher.Compare(s.dummyHash, password)
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	token, exp, err := s.tokens.Issue(user)
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}
	return &port.LoginResult{AccessToken: token, TokenType: "Bearer", ExpiresAt: exp, User: user}, nil
}

// Authenticate verifies a token and checks that its subject still exists, so
// tokens of deleted users stop working immediately.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*port.TokenClaims, error) {
	claims, err := s.tokens.Verify(token)
	if err != nil {
		return nil, domain.ErrInvalidToken
	}
	if _, err := s.repo.GetByID(ctx, claims.UserID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidToken
		}
		return nil, err
	}
	return claims, nil
}
