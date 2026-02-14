// internal/adapters/repo/stomongo/findOne.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

func FindOne(ctx context.Context, collection string, filter bson.M, out any) error {
	return coll(collection).FindOne(ctx, filter).Decode(out)
}
