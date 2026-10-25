package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// MemoryIdempotencyStore is an in-memory fake of port.IdempotencyStore with
// the same claim semantics as the MongoDB implementation. Safe for concurrent
// use.
type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]port.IdempotencyRecord
}

var _ port.IdempotencyStore = (*MemoryIdempotencyStore)(nil)

// NewMemoryIdempotencyStore returns an empty store.
func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{records: make(map[string]port.IdempotencyRecord)}
}

func idemKey(scope, key string) string { return scope + "\n" + key }

func (s *MemoryIdempotencyStore) Claim(_ context.Context, rec port.IdempotencyRecord, staleBefore time.Time) (*port.IdempotencyRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := idemKey(rec.Scope, rec.Key)
	existing, ok := s.records[k]
	if !ok || (!existing.Completed && existing.RequestHash == rec.RequestHash && existing.ClaimedAt.Before(staleBefore)) {
		s.records[k] = rec
		return nil, true, nil
	}
	return &existing, false, nil
}

func (s *MemoryIdempotencyStore) Complete(_ context.Context, scope, key string, status int, location string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := idemKey(scope, key)
	rec := s.records[k]
	rec.Completed, rec.StatusCode, rec.Location, rec.Body = true, status, location, append([]byte(nil), body...)
	s.records[k] = rec
	return nil
}

func (s *MemoryIdempotencyStore) Release(_ context.Context, scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[idemKey(scope, key)]; ok && !rec.Completed {
		delete(s.records, idemKey(scope, key))
	}
	return nil
}
