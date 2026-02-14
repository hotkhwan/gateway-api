// internal/adapters/repo/stomongo/external.go
package stomongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func SetExternalID(ctx context.Context, collection string, id primitive.ObjectID, ns string, idVal any) error {
	fields := bson.M{
		"external." + ns + ".id":       idVal,
		"external." + ns + ".syncedAt": time.Now(),
	}
	_, err := UpdateByID(ctx, collection, id, fields)
	return err
}

func SetExternalIDByKey(ctx context.Context, collection, key string, value any, ns string, idVal any) error {
	fields := bson.M{
		"external." + ns + ".id":       idVal,
		"external." + ns + ".syncedAt": time.Now(),
	}
	_, err := UpdateOne(ctx, collection, bson.M{key: value}, fields)
	return err
}
