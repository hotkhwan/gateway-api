package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

func CreateIndexes(ctx context.Context, collection string, models []mongo.IndexModel) error {
	_, err := coll(collection).Indexes().CreateMany(ctx, models)
	return err
}
