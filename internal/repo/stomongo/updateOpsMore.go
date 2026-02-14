// internal/adapters/repo/stomongo/updateOpsMore.go
package stomongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// UpdateManyOps: $set/$unset พร้อมเติม updatedAt เมื่อมี $set
func UpdateManyOps(ctx context.Context, collection string, filter bson.M, set bson.M, unset bson.M) (*mongo.UpdateResult, error) {
	update := bson.M{}
	if len(set) > 0 {
		update["$set"] = nowSet(set)
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	return coll(collection).UpdateMany(ctx, filter, update)
}

// UpdateOneUnsetOnlyTouch: เคสที่มีแต่ $unset แต่ยังอยากให้ updatedAt ขยับ
func UpdateOneUnsetOnlyTouch(ctx context.Context, collection string, filter bson.M, unset bson.M) (*mongo.UpdateResult, error) {
	update := bson.M{
		"$unset": unset,
		"$set":   bson.M{"updatedAt": time.Now().UTC()},
	}
	return coll(collection).UpdateOne(ctx, filter, update)
}

// Touch: อัปเดตแค่ updatedAt (เช่น revive flags อื่นทำที่ layer บน)
func Touch(ctx context.Context, collection string, filter bson.M) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateMany(ctx, filter, bson.M{"$set": bson.M{"updatedAt": time.Now().UTC()}})
}
