// internal/adapters/repo/stomongo/bulk.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BulkWrite: wrapper ตรงไปยัง collection.BulkWrite
func BulkWrite(ctx context.Context, collection string, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
	return coll(collection).BulkWrite(ctx, models, opts...)
}

// (ทางเลือก) ถ้าอยากได้ access ถึง *mongo.Collection โดยตรง
func WithCollection(collection string, fn func(*mongo.Collection) error) error {
	return fn(coll(collection))
}
