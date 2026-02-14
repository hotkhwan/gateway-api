// internal/adapters/repo/stomongo/indexExtra.go
package stomongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func EnsureUniqueIndex(ctx context.Context, collection string, keys bson.D, name string) error {
	model := mongo.IndexModel{
		Keys:    keys,
		Options: options.Index().SetName(name).SetUnique(true),
	}
	_, err := coll(collection).Indexes().CreateOne(ctx, model)
	return err
}

func EnsureTTLIndex(ctx context.Context, collection string, keys bson.D, name string, expireAfter time.Duration) error {
	sec := int32(expireAfter.Seconds())
	model := mongo.IndexModel{
		Keys:    keys,
		Options: options.Index().SetName(name).SetExpireAfterSeconds(sec),
	}
	_, err := coll(collection).Indexes().CreateOne(ctx, model)
	return err
}
