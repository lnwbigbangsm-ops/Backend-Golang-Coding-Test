// Package service implements the application use cases on top of the ports.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// UserService implements port.UserService.
type UserService struct {
	repo   port.UserRepository
	hasher port.PasswordHasher
	now    func() time.Time
}

var _ port.UserService = (*UserService)(nil)

// NewUserService builds a UserService.
func NewUserService(repo port.UserRepository, hasher port.PasswordHasher) *UserService {
	return &UserService{
		repo:   repo,
		hasher: hasher,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Create validates the input, hashes the password and stores a new user.
// Email uniqueness is enforced by the repository so concurrent requests
// cannot both succeed.
func (s *UserService) Create(ctx context.Context, in port.CreateUserInput) (*domain.User, error) {
	name := domain.NormalizeName(in.Name)
	email := domain.NormalizeEmail(in.Email)

	verr := &domain.ValidationError{}
	verr.Add("name", domain.ValidateName(name))
	verr.Add("email", domain.ValidateEmail(email))
	verr.Add("password", domain.ValidatePassword(in.Password))
	if err := verr.OrNil(); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    s.now().Truncate(time.Millisecond), // MongoDB stores millisecond precision
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Get returns the user with the given ID.
func (s *UserService) Get(ctx context.Context, id string) (*domain.User, error) {
	if id == "" {
		return nil, domain.ErrUserNotFound
	}
	return s.repo.GetByID(ctx, id)
}

// List returns all users.
func (s *UserService) List(ctx context.Context) ([]*domain.User, error) {
	return s.repo.List(ctx)
}

// Update changes a user's name and/or email.
func (s *UserService) Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error) {
	if id == "" {
		return nil, domain.ErrUserNotFound
	}

	verr := &domain.ValidationError{}
	if update.Name == nil && update.Email == nil {
		verr.Add("body", "at least one of name or email is required")
		return nil, verr
	}
	if update.Name != nil {
		name := domain.NormalizeName(*update.Name)
		update.Name = &name
		verr.Add("name", domain.ValidateName(name))
	}
	if update.Email != nil {
		email := domain.NormalizeEmail(*update.Email)
		update.Email = &email
		verr.Add("email", domain.ValidateEmail(email))
	}
	if err := verr.OrNil(); err != nil {
		return nil, err
	}

	return s.repo.Update(ctx, id, update)
}

// Delete removes a user.
func (s *UserService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrUserNotFound
	}
	return s.repo.Delete(ctx, id)
}

// Count returns the total number of users.
func (s *UserService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}
