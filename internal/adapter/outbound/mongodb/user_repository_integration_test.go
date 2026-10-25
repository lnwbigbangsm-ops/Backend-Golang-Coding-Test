package mongodb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
)

// TestUserRepository_Integration runs against a real MongoDB and is skipped
// unless MONGO_TEST_URI is set, e.g.:
//
//	MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/adapter/outbound/mongodb/
func TestUserRepository_Integration(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("MONGO_TEST_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database(fmt.Sprintf("user_mgmt_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo := NewUserRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	alice := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: "h", CreatedAt: now}
	if err := repo.Create(ctx, alice); err != nil || alice.ID == "" {
		t.Fatalf("Create: id=%q err=%v", alice.ID, err)
	}
	dup := &domain.User{Name: "A2", Email: "alice@example.com", PasswordHash: "h", CreatedAt: now}
	if err := repo.Create(ctx, dup); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Errorf("duplicate Create err = %v", err)
	}

	got, err := repo.GetByID(ctx, alice.ID)
	if err != nil || got.Email != alice.Email || !got.CreatedAt.Equal(now) {
		t.Errorf("GetByID = %+v, %v", got, err)
	}
	if _, err := repo.GetByID(ctx, "not-an-object-id"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("GetByID(invalid) err = %v", err)
	}
	if _, err := repo.GetByEmail(ctx, "alice@example.com"); err != nil {
		t.Errorf("GetByEmail err = %v", err)
	}

	bob := &domain.User{Name: "Bob", Email: "bob@example.com", PasswordHash: "h", CreatedAt: now.Add(time.Second)}
	if err := repo.Create(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if users, err := repo.List(ctx); err != nil || len(users) != 2 || users[0].ID != alice.ID {
		t.Errorf("List = %v, %v", users, err)
	}

	newName := "Robert"
	if u, err := repo.Update(ctx, bob.ID, domain.UserUpdate{Name: &newName}); err != nil || u.Name != "Robert" {
		t.Errorf("Update = %+v, %v", u, err)
	}
	taken := "alice@example.com"
	if _, err := repo.Update(ctx, bob.ID, domain.UserUpdate{Email: &taken}); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Errorf("Update to taken email err = %v", err)
	}

	if n, err := repo.Count(ctx); err != nil || n != 2 {
		t.Errorf("Count = %d, %v", n, err)
	}
	if err := repo.Delete(ctx, bob.ID); err != nil {
		t.Errorf("Delete err = %v", err)
	}
	if err := repo.Delete(ctx, bob.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("second Delete err = %v", err)
	}
}
