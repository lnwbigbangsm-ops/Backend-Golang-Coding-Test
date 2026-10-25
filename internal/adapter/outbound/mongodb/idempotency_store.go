package mongodb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// IdempotencyCollectionName is the collection idempotency keys are stored in.
const IdempotencyCollectionName = "idempotency_keys"

type idempotencyDocument struct {
	// ID is a hash of scope and key, so the unique _id index is what decides
	// which of several concurrent claims wins.
	ID          string    `bson:"_id"`
	Scope       string    `bson:"scope"`
	Key         string    `bson:"key"`
	RequestHash string    `bson:"request_hash"`
	ClaimedAt   time.Time `bson:"claimed_at"`
	CreatedAt   time.Time `bson:"created_at"`
	Completed   bool      `bson:"completed"`
	StatusCode  int       `bson:"status_code,omitempty"`
	Location    string    `bson:"location,omitempty"`
	Body        []byte    `bson:"body,omitempty"`
}

func (d *idempotencyDocument) toRecord() *port.IdempotencyRecord {
	return &port.IdempotencyRecord{
		Scope: d.Scope, Key: d.Key, RequestHash: d.RequestHash, ClaimedAt: d.ClaimedAt.UTC(),
		Completed: d.Completed, StatusCode: d.StatusCode, Location: d.Location, Body: d.Body,
	}
}

// IdempotencyStore implements port.IdempotencyStore.
type IdempotencyStore struct {
	coll *mongo.Collection
}

var _ port.IdempotencyStore = (*IdempotencyStore)(nil)

// NewIdempotencyStore returns a store backed by db.idempotency_keys.
func NewIdempotencyStore(db *mongo.Database) *IdempotencyStore {
	return &IdempotencyStore{coll: db.Collection(IdempotencyCollectionName)}
}

// EnsureIndexes creates a TTL index so keys are removed after retention.
func (s *IdempotencyStore) EnsureIndexes(ctx context.Context, retention time.Duration) error {
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: 1}},
		Options: options.Index().SetName("ttl_created_at").SetExpireAfterSeconds(int32(retention / time.Second)),
	})
	if err != nil {
		return fmt.Errorf("create idempotency ttl index: %w", err)
	}
	return nil
}

func idempotencyID(scope, key string) string {
	sum := sha256.Sum256([]byte(scope + "\n" + key))
	return hex.EncodeToString(sum[:])
}

func (s *IdempotencyStore) Claim(ctx context.Context, rec port.IdempotencyRecord, staleBefore time.Time) (*port.IdempotencyRecord, bool, error) {
	id := idempotencyID(rec.Scope, rec.Key)
	claimedAt := rec.ClaimedAt.UTC().Truncate(time.Millisecond)

	// Two attempts: the existing record can expire (TTL) between a failed
	// insert and the read that follows it.
	for range 2 {
		_, err := s.coll.InsertOne(ctx, idempotencyDocument{
			ID: id, Scope: rec.Scope, Key: rec.Key, RequestHash: rec.RequestHash,
			ClaimedAt: claimedAt, CreatedAt: claimedAt,
		})
		if err == nil {
			return nil, true, nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			return nil, false, fmt.Errorf("claim idempotency key: %w", err)
		}

		var existing idempotencyDocument
		if err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&existing); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			return nil, false, fmt.Errorf("read idempotency key: %w", err)
		}

		// Take over an abandoned claim (for example after a crash). The filter
		// on the observed claimed_at makes the takeover itself atomic.
		if !existing.Completed && existing.RequestHash == rec.RequestHash && existing.ClaimedAt.Before(staleBefore) {
			res, err := s.coll.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: id}, {Key: "completed", Value: false}, {Key: "claimed_at", Value: existing.ClaimedAt}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "claimed_at", Value: claimedAt}}}})
			if err != nil {
				return nil, false, fmt.Errorf("take over idempotency key: %w", err)
			}
			if res.ModifiedCount == 1 {
				return nil, true, nil
			}
		}
		return existing.toRecord(), false, nil
	}
	return nil, false, errors.New("claim idempotency key: record changed concurrently")
}

func (s *IdempotencyStore) Complete(ctx context.Context, scope, key string, status int, location string, body []byte) error {
	_, err := s.coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: idempotencyID(scope, key)}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "completed", Value: true},
			{Key: "status_code", Value: status},
			{Key: "location", Value: location},
			{Key: "body", Value: body},
		}}})
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

func (s *IdempotencyStore) Release(ctx context.Context, scope, key string) error {
	_, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: idempotencyID(scope, key)}, {Key: "completed", Value: false}})
	if err != nil {
		return fmt.Errorf("release idempotency key: %w", err)
	}
	return nil
}
