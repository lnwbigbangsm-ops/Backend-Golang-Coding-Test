package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/mocks"
)

var fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)

func newTestUserService(repo port.UserRepository, hasher port.PasswordHasher) *UserService {
	s := NewUserService(repo, hasher)
	s.now = func() time.Time { return fixedNow }
	return s
}

func ptr[T any](v T) *T { return &v }

func TestUserService_Create(t *testing.T) {
	var stored *domain.User
	repo := &mocks.UserRepository{CreateFn: func(_ context.Context, u *domain.User) error {
		u.ID = "id-1"
		stored = u
		return nil
	}}
	s := newTestUserService(repo, &mocks.PasswordHasher{})

	user, err := s.Create(context.Background(), port.CreateUserInput{
		Name: "  Alice ", Email: " Alice@Example.com ", Password: "password123",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if stored == nil {
		t.Fatal("repository Create was not called")
	}
	if user.ID != "id-1" || user.Name != "Alice" || user.Email != "alice@example.com" {
		t.Errorf("unexpected user: %+v", user)
	}
	if user.PasswordHash != "hashed:password123" {
		t.Errorf("password was not hashed: %q", user.PasswordHash)
	}
	if want := fixedNow.Truncate(time.Millisecond); !user.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", user.CreatedAt, want)
	}
}

func TestUserService_Create_ValidationSkipsRepository(t *testing.T) {
	s := newTestUserService(&mocks.UserRepository{}, &mocks.PasswordHasher{}) // any repo call panics

	_, err := s.Create(context.Background(), port.CreateUserInput{Name: "", Email: "bad", Password: "short"})

	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	for _, f := range []string{"name", "email", "password"} {
		if verr.Fields[f] == "" {
			t.Errorf("missing validation message for %q", f)
		}
	}
}

func TestUserService_Create_Errors(t *testing.T) {
	valid := port.CreateUserInput{Name: "Alice", Email: "alice@example.com", Password: "password123"}

	t.Run("hasher failure", func(t *testing.T) {
		boom := errors.New("boom")
		s := newTestUserService(&mocks.UserRepository{}, &mocks.PasswordHasher{HashErr: boom})
		if _, err := s.Create(context.Background(), valid); !errors.Is(err, boom) {
			t.Errorf("err = %v, want wrapped boom", err)
		}
	})

	t.Run("duplicate email", func(t *testing.T) {
		repo := &mocks.UserRepository{CreateFn: func(context.Context, *domain.User) error {
			return domain.ErrEmailAlreadyExists
		}}
		s := newTestUserService(repo, &mocks.PasswordHasher{})
		if _, err := s.Create(context.Background(), valid); !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Errorf("err = %v, want ErrEmailAlreadyExists", err)
		}
	})
}

func TestUserService_Get(t *testing.T) {
	repo := &mocks.UserRepository{GetByIDFn: func(_ context.Context, id string) (*domain.User, error) {
		if id == "id-1" {
			return &domain.User{ID: id}, nil
		}
		return nil, domain.ErrUserNotFound
	}}
	s := newTestUserService(repo, &mocks.PasswordHasher{})

	if u, err := s.Get(context.Background(), "id-1"); err != nil || u.ID != "id-1" {
		t.Errorf("Get(id-1) = %v, %v", u, err)
	}
	if _, err := s.Get(context.Background(), "missing"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Get(missing) err = %v", err)
	}
	if _, err := s.Get(context.Background(), ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Get(\"\") err = %v", err)
	}
}

func TestUserService_List(t *testing.T) {
	repo := &mocks.UserRepository{ListFn: func(context.Context) ([]*domain.User, error) {
		return []*domain.User{{ID: "1"}, {ID: "2"}}, nil
	}}
	users, err := newTestUserService(repo, &mocks.PasswordHasher{}).List(context.Background())
	if err != nil || len(users) != 2 {
		t.Errorf("List = %v, %v", users, err)
	}
}

func TestUserService_Update(t *testing.T) {
	var got domain.UserUpdate
	repo := &mocks.UserRepository{UpdateFn: func(_ context.Context, id string, u domain.UserUpdate) (*domain.User, error) {
		got = u
		return &domain.User{ID: id, Name: *u.Name, Email: *u.Email}, nil
	}}
	s := newTestUserService(repo, &mocks.PasswordHasher{})

	user, err := s.Update(context.Background(), "id-1", domain.UserUpdate{Name: ptr(" Bob "), Email: ptr(" BOB@Example.com")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if *got.Name != "Bob" || *got.Email != "bob@example.com" {
		t.Errorf("update not normalized: name=%q email=%q", *got.Name, *got.Email)
	}
	if user.Email != "bob@example.com" {
		t.Errorf("returned user = %+v", user)
	}
}

func TestUserService_Update_Validation(t *testing.T) {
	s := newTestUserService(&mocks.UserRepository{}, &mocks.PasswordHasher{})
	tests := []struct {
		name   string
		update domain.UserUpdate
		field  string
	}{
		{"no fields", domain.UserUpdate{}, "body"},
		{"blank name", domain.UserUpdate{Name: ptr("   ")}, "name"},
		{"bad email", domain.UserUpdate{Email: ptr("nope")}, "email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Update(context.Background(), "id-1", tt.update)
			var verr *domain.ValidationError
			if !errors.As(err, &verr) || verr.Fields[tt.field] == "" {
				t.Errorf("err = %v, want validation error on %q", err, tt.field)
			}
		})
	}
}

func TestUserService_DeleteAndCount(t *testing.T) {
	deleted := ""
	repo := &mocks.UserRepository{
		DeleteFn: func(_ context.Context, id string) error { deleted = id; return nil },
		CountFn:  func(context.Context) (int64, error) { return 42, nil },
	}
	s := newTestUserService(repo, &mocks.PasswordHasher{})

	if err := s.Delete(context.Background(), "id-1"); err != nil || deleted != "id-1" {
		t.Errorf("Delete: err=%v deleted=%q", err, deleted)
	}
	if err := s.Delete(context.Background(), ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Delete(\"\") err = %v", err)
	}
	if n, err := s.Count(context.Background()); err != nil || n != 42 {
		t.Errorf("Count = %d, %v", n, err)
	}
}
