// internal/repo/stomongo/aggregate.go
package stomongo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"

	"go.mongodb.org/mongo-driver/mongo"
)

// Aggregate runs an aggregation pipeline on the given collection
// and decodes all results into `out` (which should be a pointer to a slice).
func Aggregate(ctx context.Context, collection string, pipeline mongo.Pipeline, out interface{}) error {
	coll := config.DB.Collection(collection)

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	return cur.All(ctx, out)
}
