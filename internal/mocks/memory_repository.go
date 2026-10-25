package mocks

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"backend-golang-coding-test/internal/core/domain"
	"backend-golang-coding-test/internal/core/port"
)

// MemoryUserRepository is an in-memory fake of port.UserRepository that
// mimics MongoDB behaviour (generated IDs, unique email). Safe for
// concurrent use.
type MemoryUserRepository struct {
	mu     sync.RWMutex
	nextID int
	users  map[string]domain.User
}

var _ port.UserRepository = (*MemoryUserRepository)(nil)

// NewMemoryUserRepository returns an empty repository.
func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{users: make(map[string]domain.User)}
}

func (r *MemoryUserRepository) Create(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emailTaken(user.Email, "") {
		return domain.ErrEmailAlreadyExists
	}
	r.nextID++
	user.ID = fmt.Sprintf("%024x", r.nextID)
	r.users[user.ID] = *user
	return nil
}

func (r *MemoryUserRepository) GetByID(_ context.Context, id string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return &u, nil
}

func (r *MemoryUserRepository) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (r *MemoryUserRepository) List(_ context.Context) ([]*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, &u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryUserRepository) Update(_ context.Context, id string, update domain.UserUpdate) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	if update.Email != nil && r.emailTaken(*update.Email, id) {
		return nil, domain.ErrEmailAlreadyExists
	}
	if update.Name != nil {
		u.Name = *update.Name
	}
	if update.Email != nil {
		u.Email = *update.Email
	}
	r.users[id] = u
	return &u, nil
}

func (r *MemoryUserRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(r.users, id)
	return nil
}

func (r *MemoryUserRepository) Count(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.users)), nil
}

func (r *MemoryUserRepository) emailTaken(email, exceptID string) bool {
	for id, u := range r.users {
		if u.Email == email && id != exceptID {
			return true
		}
	}
	return false
}
