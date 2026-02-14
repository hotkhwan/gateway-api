// internal/services/crimes/indexes.go
package crimes

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func EnsureCrimesIndexes(ctx context.Context, coll *mongo.Collection) error {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// ถ้ามี index เก่าค้าง ให้ลองลบทิ้งก่อน (ถ้าไม่มี จะได้ raw=nil, err=nil ก็ไม่เป็นไร)
	_, _ = coll.Indexes().DropOne(c, "uniq_person_source")
	_, _ = coll.Indexes().DropOne(c, "uniq_crimes_person")

	models := []mongo.IndexModel{
		// ✅ unique เฉพาะเอกสารที่ source="crimes"
		{
			Keys: bson.D{{Key: "personKey", Value: 1}, {Key: "source", Value: 1}},
			Options: options.Index().
				SetName("uniq_crimes_person").
				SetUnique(true).
				SetPartialFilterExpression(bson.M{"source": "crimes"}),
		},
		// ใช้กับ sweep: {source:"crimes", seenAt:{$lt:runAt}}
		{
			Keys:    bson.D{{Key: "source", Value: 1}, {Key: "seenAt", Value: 1}},
			Options: options.Index().SetName("idx_source_seenAt"),
		},
		// เผื่อ query รายตัว
		{
			Keys:    bson.D{{Key: "source", Value: 1}, {Key: "personKey", Value: 1}},
			Options: options.Index().SetName("idx_source_personKey"),
		},
	}

	_, err := coll.Indexes().CreateMany(c, models)
	return err
}
