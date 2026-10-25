package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend-golang-coding-test/internal/core/domain"
	"backend-golang-coding-test/internal/core/port"
	"backend-golang-coding-test/internal/mocks"
)

var alice = &domain.User{ID: "id-1", Name: "Alice", Email: "alice@example.com", PasswordHash: "hashed:password123"}

func newTestAuthService(t *testing.T, repo port.UserRepository, hasher *mocks.PasswordHasher, tokens port.TokenService) *AuthService {
	t.Helper()
	s, err := NewAuthService(NewUserService(repo, hasher), repo, hasher, tokens)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	return s
}

func repoWithAlice() *mocks.UserRepository {
	return &mocks.UserRepository{
		GetByEmailFn: func(_ context.Context, email string) (*domain.User, error) {
			if email == alice.Email {
				return alice, nil
			}
			return nil, domain.ErrUserNotFound
		},
		GetByIDFn: func(_ context.Context, id string) (*domain.User, error) {
			if id == alice.ID {
				return alice, nil
			}
			return nil, domain.ErrUserNotFound
		},
	}
}

func TestAuthService_Login(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	tokens := &mocks.TokenService{IssueFn: func(u *domain.User) (string, time.Time, error) {
		return "token-for-" + u.ID, exp, nil
	}}
	s := newTestAuthService(t, repoWithAlice(), &mocks.PasswordHasher{}, tokens)

	res, err := s.Login(context.Background(), " ALICE@example.com ", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.AccessToken != "token-for-id-1" || res.TokenType != "Bearer" || !res.ExpiresAt.Equal(exp) || res.User.ID != "id-1" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	tests := []struct {
		name, email, password string
	}{
		{"wrong password", "alice@example.com", "wrong-password"},
		{"unknown email", "nobody@example.com", "password123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasher := &mocks.PasswordHasher{}
			s := newTestAuthService(t, repoWithAlice(), hasher, &mocks.TokenService{}) // Issue must not be called

			_, err := s.Login(context.Background(), tt.email, tt.password)
			if !errors.Is(err, domain.ErrInvalidCredentials) {
				t.Errorf("err = %v, want ErrInvalidCredentials", err)
			}
			// Both paths must run a hash comparison so response time does not
			// reveal whether the email exists.
			if hasher.CompareCalls != 1 {
				t.Errorf("Compare called %d times, want 1", hasher.CompareCalls)
			}
		})
	}
}

func TestAuthService_Login_Validation(t *testing.T) {
	s := newTestAuthService(t, &mocks.UserRepository{}, &mocks.PasswordHasher{}, &mocks.TokenService{})
	_, err := s.Login(context.Background(), " ", "")
	var verr *domain.ValidationError
	if !errors.As(err, &verr) || verr.Fields["email"] == "" || verr.Fields["password"] == "" {
		t.Errorf("err = %v, want validation errors on email and password", err)
	}
}

func TestAuthService_Login_RepositoryError(t *testing.T) {
	boom := errors.New("db down")
	repo := &mocks.UserRepository{GetByEmailFn: func(context.Context, string) (*domain.User, error) { return nil, boom }}
	s := newTestAuthService(t, repo, &mocks.PasswordHasher{}, &mocks.TokenService{})
	if _, err := s.Login(context.Background(), "alice@example.com", "password123"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want db error", err)
	}
}

func TestAuthService_Register(t *testing.T) {
	repo := &mocks.UserRepository{CreateFn: func(_ context.Context, u *domain.User) error { u.ID = "new"; return nil }}
	s := newTestAuthService(t, repo, &mocks.PasswordHasher{}, &mocks.TokenService{})
	u, err := s.Register(context.Background(), port.CreateUserInput{Name: "Bob", Email: "bob@example.com", Password: "password123"})
	if err != nil || u.ID != "new" {
		t.Errorf("Register = %+v, %v", u, err)
	}
}

func TestAuthService_Authenticate(t *testing.T) {
	tokens := &mocks.TokenService{VerifyFn: func(token string) (*port.TokenClaims, error) {
		switch token {
		case "good":
			return &port.TokenClaims{UserID: alice.ID}, nil
		case "deleted-user":
			return &port.TokenClaims{UserID: "gone"}, nil
		}
		return nil, domain.ErrInvalidToken
	}}
	s := newTestAuthService(t, repoWithAlice(), &mocks.PasswordHasher{}, tokens)

	if c, err := s.Authenticate(context.Background(), "good"); err != nil || c.UserID != alice.ID {
		t.Errorf("Authenticate(good) = %+v, %v", c, err)
	}
	for _, tok := range []string{"bad", "deleted-user"} {
		if _, err := s.Authenticate(context.Background(), tok); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("Authenticate(%s) err = %v, want ErrInvalidToken", tok, err)
		}
	}
}
