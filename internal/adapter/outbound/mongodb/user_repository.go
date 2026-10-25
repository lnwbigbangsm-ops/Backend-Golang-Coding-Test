// Package mongodb implements the user repository on MongoDB using the
// official Go driver.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// CollectionName is the collection users are stored in.
const CollectionName = "users"

// userDocument is the persistence model; it keeps BSON concerns out of the
// domain entity.
type userDocument struct {
	ID           bson.ObjectID `bson:"_id,omitempty"`
	Name         string        `bson:"name"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    time.Time     `bson:"created_at"`
	CreatedBy    string        `bson:"created_by,omitempty"`
}

func (d *userDocument) toDomain() *domain.User {
	return &domain.User{
		ID:           d.ID.Hex(),
		Name:         d.Name,
		Email:        d.Email,
		PasswordHash: d.PasswordHash,
		CreatedAt:    d.CreatedAt.UTC(),
		CreatedBy:    d.CreatedBy,
	}
}

// UserRepository implements port.UserRepository.
type UserRepository struct {
	coll *mongo.Collection
}

var _ port.UserRepository = (*UserRepository)(nil)

// NewUserRepository returns a repository backed by db.users.
func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{coll: db.Collection(CollectionName)}
}

// EnsureIndexes creates the unique email index and the index that backs the
// list order. It is idempotent.
func (r *UserRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uniq_email")},
		{Keys: bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("created_at_id")},
	})
	if err != nil {
		return fmt.Errorf("create user indexes: %w", err)
	}
	return nil
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	doc := userDocument{
		ID:           bson.NewObjectID(),
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		CreatedAt:    user.CreatedAt,
		CreatedBy:    user.CreatedBy,
	}
	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrEmailAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	user.ID = doc.ID.Hex()
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}
	return r.findOne(ctx, bson.D{{Key: "_id", Value: oid}})
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.D) (*domain.User, error) {
	var doc userDocument
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]*domain.User, error) {
	// _id breaks ties between users created in the same millisecond, so the
	// order (and therefore paging) is stable.
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(offset)).
		SetLimit(int64(limit))
	cur, err := r.coll.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	var docs []userDocument
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}
	users := make([]*domain.User, len(docs))
	for i := range docs {
		users[i] = docs[i].toDomain()
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}
	set := bson.D{}
	if update.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *update.Name})
	}
	if update.Email != nil {
		set = append(set, bson.E{Key: "email", Value: *update.Email})
	}
	if len(set) == 0 {
		return r.GetByID(ctx, id)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var doc userDocument
	err = r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: oid}}, bson.D{{Key: "$set", Value: set}}, opts).Decode(&doc)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return nil, domain.ErrUserNotFound
	case mongo.IsDuplicateKeyError(err):
		return nil, domain.ErrEmailAlreadyExists
	case err != nil:
		return nil, fmt.Errorf("update user: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrUserNotFound
	}
	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	n, err := r.coll.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}
