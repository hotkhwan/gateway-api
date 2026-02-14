// internal/adapters/repo/stomongo/bulkExtras.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func BulkWriteUnordered(ctx context.Context, collection string, models []mongo.WriteModel) (*mongo.BulkWriteResult, error) {
	return coll(collection).BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
}

func BulkWriteOrdered(ctx context.Context, collection string, models []mongo.WriteModel) (*mongo.BulkWriteResult, error) {
	return coll(collection).BulkWrite(ctx, models, options.BulkWrite().SetOrdered(true))
}
