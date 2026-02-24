// internal/repo/devicerepo/deviceGroupUpdate.go
package devicerepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Update — name / description / mapVisibility
// groupId = ObjectID hex
func (r *ResourceGroupRepo) Update(ctx context.Context, groupId, name, description, mapVisibility string) error {
	oid, err := primitive.ObjectIDFromHex(groupId)
	if err != nil {
		return ErrNotFound
	}

	setFields := bson.M{
		"name":      name,
		"updatedAt": time.Now(),
	}
	if description != "" {
		setFields["description"] = description
	}
	if mapVisibility != "" {
		setFields["mapVisibility"] = mapVisibility
	}

	res, err := stomongo.UpdateOne(ctx, r.collection,
		bson.M{"_id": oid},
		setFields,
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
