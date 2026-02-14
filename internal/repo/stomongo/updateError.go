// internal/adapters/repo/stomongo/updateError.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo/options"
)

func UpdateOneError(ctx context.Context, collection string, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (int64, int64, error) {
	res, err := coll(collection).UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		return 0, 0, err
	}
	return res.MatchedCount, res.ModifiedCount, nil
}
