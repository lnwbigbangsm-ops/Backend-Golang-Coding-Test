// Package mocks provides hand-written test doubles for the ports. Function
// field mocks panic when an unexpected method is called, so tests fail loudly
// if code touches a dependency it should not.
package mocks

import (
	"context"
	"time"

	"backend-golang-coding-test/internal/core/domain"
	"backend-golang-coding-test/internal/core/port"
)

// UserRepository is a function-field mock of port.UserRepository.
type UserRepository struct {
	CreateFn     func(ctx context.Context, user *domain.User) error
	GetByIDFn    func(ctx context.Context, id string) (*domain.User, error)
	GetByEmailFn func(ctx context.Context, email string) (*domain.User, error)
	ListFn       func(ctx context.Context) ([]*domain.User, error)
	UpdateFn     func(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	DeleteFn     func(ctx context.Context, id string) error
	CountFn      func(ctx context.Context) (int64, error)
}

var _ port.UserRepository = (*UserRepository)(nil)

func (m *UserRepository) Create(ctx context.Context, user *domain.User) error {
	must(m.CreateFn != nil, "UserRepository.Create")
	return m.CreateFn(ctx, user)
}

func (m *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	must(m.GetByIDFn != nil, "UserRepository.GetByID")
	return m.GetByIDFn(ctx, id)
}

func (m *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	must(m.GetByEmailFn != nil, "UserRepository.GetByEmail")
	return m.GetByEmailFn(ctx, email)
}

func (m *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	must(m.ListFn != nil, "UserRepository.List")
	return m.ListFn(ctx)
}

func (m *UserRepository) Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error) {
	must(m.UpdateFn != nil, "UserRepository.Update")
	return m.UpdateFn(ctx, id, update)
}

func (m *UserRepository) Delete(ctx context.Context, id string) error {
	must(m.DeleteFn != nil, "UserRepository.Delete")
	return m.DeleteFn(ctx, id)
}

func (m *UserRepository) Count(ctx context.Context) (int64, error) {
	must(m.CountFn != nil, "UserRepository.Count")
	return m.CountFn(ctx)
}

// PasswordHasher is a deterministic, fast stand-in for bcrypt:
// Hash(p) == "hashed:"+p.
type PasswordHasher struct {
	HashErr      error
	CompareCalls int
}

var _ port.PasswordHasher = (*PasswordHasher)(nil)

func (h *PasswordHasher) Hash(password string) (string, error) {
	if h.HashErr != nil {
		return "", h.HashErr
	}
	return "hashed:" + password, nil
}

func (h *PasswordHasher) Compare(hash, password string) error {
	h.CompareCalls++
	if hash != "hashed:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// TokenService is a function-field mock of port.TokenService.
type TokenService struct {
	IssueFn  func(user *domain.User) (string, time.Time, error)
	VerifyFn func(token string) (*port.TokenClaims, error)
}

var _ port.TokenService = (*TokenService)(nil)

func (m *TokenService) Issue(user *domain.User) (string, time.Time, error) {
	must(m.IssueFn != nil, "TokenService.Issue")
	return m.IssueFn(user)
}

func (m *TokenService) Verify(token string) (*port.TokenClaims, error) {
	must(m.VerifyFn != nil, "TokenService.Verify")
	return m.VerifyFn(token)
}

// UserService is a function-field mock of port.UserService.
type UserService struct {
	CreateFn func(ctx context.Context, in port.CreateUserInput) (*domain.User, error)
	GetFn    func(ctx context.Context, id string) (*domain.User, error)
	ListFn   func(ctx context.Context) ([]*domain.User, error)
	UpdateFn func(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	DeleteFn func(ctx context.Context, id string) error
	CountFn  func(ctx context.Context) (int64, error)
}

var _ port.UserService = (*UserService)(nil)

func (m *UserService) Create(ctx context.Context, in port.CreateUserInput) (*domain.User, error) {
	must(m.CreateFn != nil, "UserService.Create")
	return m.CreateFn(ctx, in)
}

func (m *UserService) Get(ctx context.Context, id string) (*domain.User, error) {
	must(m.GetFn != nil, "UserService.Get")
	return m.GetFn(ctx, id)
}

func (m *UserService) List(ctx context.Context) ([]*domain.User, error) {
	must(m.ListFn != nil, "UserService.List")
	return m.ListFn(ctx)
}

func (m *UserService) Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error) {
	must(m.UpdateFn != nil, "UserService.Update")
	return m.UpdateFn(ctx, id, update)
}

func (m *UserService) Delete(ctx context.Context, id string) error {
	must(m.DeleteFn != nil, "UserService.Delete")
	return m.DeleteFn(ctx, id)
}

func (m *UserService) Count(ctx context.Context) (int64, error) {
	must(m.CountFn != nil, "UserService.Count")
	return m.CountFn(ctx)
}

// AuthService is a function-field mock of port.AuthService.
type AuthService struct {
	RegisterFn     func(ctx context.Context, in port.CreateUserInput) (*domain.User, error)
	LoginFn        func(ctx context.Context, email, password string) (*port.LoginResult, error)
	AuthenticateFn func(ctx context.Context, token string) (*port.TokenClaims, error)
}

var _ port.AuthService = (*AuthService)(nil)

func (m *AuthService) Register(ctx context.Context, in port.CreateUserInput) (*domain.User, error) {
	must(m.RegisterFn != nil, "AuthService.Register")
	return m.RegisterFn(ctx, in)
}

func (m *AuthService) Login(ctx context.Context, email, password string) (*port.LoginResult, error) {
	must(m.LoginFn != nil, "AuthService.Login")
	return m.LoginFn(ctx, email, password)
}

func (m *AuthService) Authenticate(ctx context.Context, token string) (*port.TokenClaims, error) {
	must(m.AuthenticateFn != nil, "AuthService.Authenticate")
	return m.AuthenticateFn(ctx, token)
}

func must(set bool, name string) {
	if !set {
		panic("mocks: unexpected call to " + name)
	}
}
