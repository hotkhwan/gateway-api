// internal/repo/devicerepo/deviceGroupUpdate.go
// เพิ่ม Update method ใน DeviceGroupRepo
package devicerepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"go.mongodb.org/mongo-driver/bson"
)

// Update — name / description / mapVisibility
// groupId = UUID (groupId field) ไม่ใช่ _id
func (r *DeviceGroupRepo) Update(ctx context.Context, groupId, name, description, mapVisibility string) error {
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
		bson.M{
			"groupId":   groupId,
			"isDeleted": bson.M{"$ne": true},
		},
		bson.M{"$set": setFields},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
