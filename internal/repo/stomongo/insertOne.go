// internal/adapters/repo/stomongo/insertOne.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func InsertOne(ctx context.Context, collection string, doc any) (primitive.ObjectID, error) {
	res, err := coll(collection).InsertOne(ctx, doc)
	if err != nil {
		return primitive.NilObjectID, err
	}
	id, _ := res.InsertedID.(primitive.ObjectID)
	return id, nil
}
