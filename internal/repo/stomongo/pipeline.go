// internal/adapters/repo/stomongo/pipeline.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpdateOnePipeline: ใช้ aggregation pipeline เป็น update
func UpdateOnePipeline(ctx context.Context, collection string, filter interface{}, pipeline mongo.Pipeline, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateOne(ctx, filter, pipeline, opts...)
}

// UpsertOnePipeline: like UpdateOnePipeline แต่บังคับ upsert
func UpsertOnePipeline(ctx context.Context, collection string, filter interface{}, pipeline mongo.Pipeline) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateOne(ctx, filter, pipeline, options.Update().SetUpsert(true))
}
