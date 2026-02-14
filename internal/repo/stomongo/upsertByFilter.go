// internal/adapters/repo/stomongo/upsertByFilter.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func UpsertByFilter(ctx context.Context, collection string, filter bson.M, setFields bson.M, onInsert bson.M) (*mongo.UpdateResult, error) {
	update := bson.M{
		"$set":         nowSet(setFields),
		"$setOnInsert": onInsert,
	}
	return coll(collection).UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
}
