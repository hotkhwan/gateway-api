// internal/adapters/repo/stomongo/delete.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func DeleteByID(ctx context.Context, collection string, id primitive.ObjectID) (*mongo.DeleteResult, error) {
	return coll(collection).DeleteOne(ctx, bson.M{"_id": id})
}

func DeleteOne(ctx context.Context, collection string, filter bson.M) (*mongo.DeleteResult, error) {
	return coll(collection).DeleteOne(ctx, filter)
}

func DeleteMany(ctx context.Context, collection string, filter bson.M) (*mongo.DeleteResult, error) {
	return coll(collection).DeleteMany(ctx, filter)
}
