package mongodb

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// TestIdempotencyStore_Integration runs against a real MongoDB and is skipped
// unless MONGO_TEST_URI is set.
func TestIdempotencyStore_Integration(t *testing.T) {
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
	db := client.Database(fmt.Sprintf("idem_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	store := NewIdempotencyStore(db)
	if err := store.EnsureIndexes(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	rec := port.IdempotencyRecord{Scope: "POST /x u1", Key: "k", RequestHash: "h1", ClaimedAt: now}

	// Exactly one of many concurrent claims wins.
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, claimed, err := store.Claim(ctx, rec, now.Add(-time.Minute)); err != nil {
				t.Error(err)
			} else if claimed {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("%d concurrent claims won, want exactly 1", winners.Load())
	}

	existing, claimed, err := store.Claim(ctx, rec, now.Add(-time.Minute))
	if err != nil || claimed || existing.Completed || existing.RequestHash != "h1" {
		t.Errorf("in-progress claim = %+v, %v, %v", existing, claimed, err)
	}

	if err := store.Complete(ctx, rec.Scope, rec.Key, 201, "/api/v1/users/1", []byte(`{"id":"1"}`)); err != nil {
		t.Fatal(err)
	}
	existing, claimed, err = store.Claim(ctx, rec, now.Add(time.Hour)) // even "stale" completed records are never taken over
	if err != nil || claimed || !existing.Completed || existing.StatusCode != 201 || existing.Location != "/api/v1/users/1" || string(existing.Body) != `{"id":"1"}` {
		t.Errorf("completed claim = %+v, %v, %v", existing, claimed, err)
	}
	if err := store.Release(ctx, rec.Scope, rec.Key); err != nil {
		t.Fatal(err)
	}
	if existing, _, _ = store.Claim(ctx, rec, now); existing == nil || !existing.Completed {
		t.Error("Release must not delete a completed record")
	}

	// A stale in-progress claim with the same body is taken over; with a
	// different body it is not.
	stale := port.IdempotencyRecord{Scope: "POST /x u1", Key: "stale", RequestHash: "h2", ClaimedAt: now.Add(-time.Hour)}
	if _, claimed, _ := store.Claim(ctx, stale, now); !claimed {
		t.Fatal("first claim failed")
	}
	other := stale
	other.RequestHash, other.ClaimedAt = "different", now
	if _, claimed, _ := store.Claim(ctx, other, now.Add(-time.Minute)); claimed {
		t.Error("stale claim taken over by a different body")
	}
	retry := stale
	retry.ClaimedAt = now
	if _, claimed, _ := store.Claim(ctx, retry, now.Add(-time.Minute)); !claimed {
		t.Error("stale claim with same body was not taken over")
	}

	// Release of an in-progress claim allows a fresh claim.
	if err := store.Release(ctx, retry.Scope, retry.Key); err != nil {
		t.Fatal(err)
	}
	if _, claimed, _ := store.Claim(ctx, retry, now.Add(-time.Minute)); !claimed {
		t.Error("claim after Release failed")
	}
}
