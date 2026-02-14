// internal/repo/authzrepo/idempotency.go
package authzrepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	idempotencyCollection = "idempotency_keys"
)

type IdempotencyKey struct {
	Key       string      `bson:"_id"`
	Response  interface{} `bson:"response"`
	CreatedAt time.Time   `bson:"createdAt"`
}

// IdempotencyRepo handles idempotency key persistence operations
type IdempotencyRepo interface {
	Get(ctx context.Context, key string) (*IdempotencyKey, error)
	Put(ctx context.Context, key string, response interface{}) error
}

type idempotencyRepo struct {
	coll *mongo.Collection
}

// NewIdempotencyRepo creates a new idempotency repository
func NewIdempotencyRepo(db *mongo.Database) IdempotencyRepo {
	// Create TTL index to auto-expire old keys after 24h
	idx := mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(24 * 60 * 60),
	}
	_, _ = db.Collection(idempotencyCollection).Indexes().CreateOne(context.Background(), idx)

	return &idempotencyRepo{
		coll: db.Collection(idempotencyCollection),
	}
}

func (r *idempotencyRepo) Get(ctx context.Context, key string) (*IdempotencyKey, error) {
	var record IdempotencyKey
	err := r.coll.FindOne(ctx, bson.M{"_id": key}).Decode(&record)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &record, err
}

func (r *idempotencyRepo) Put(ctx context.Context, key string, response interface{}) error {
	record := IdempotencyKey{
		Key:       key,
		Response:  response,
		CreatedAt: time.Now().UTC(),
	}
	_, err := r.coll.InsertOne(ctx, record)
	return err
}
