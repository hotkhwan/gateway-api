// internal/services/grpsvc/update.go
package grpsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/grpmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func UpdateGroup(ctx context.Context, id string, req grpmod.GroupRequest) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/grpsvc", "grpsvc.UpdateGroup", "grpsvc", "UpdateGroup")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("groups")

	// ตรวจว่ามีกลุ่มเป้าหมายไหม
	filter, err := idFilter(id)
	if err != nil {
		return err
	}
	var exists bson.M
	if err := coll.FindOne(ctx, filter).Decode(&exists); err != nil {
		return gmod.Errorf("GROUP_NOT_FOUND", "group not found")
	}

	// ถ้ามี parentId: ตรวจ parent มีจริง และไม่วนลูป
	if req.ParentID != nil && *req.ParentID != "" {
		parentFilter, err := idFilter(*req.ParentID)
		if err != nil {
			return gmod.Errorf("PARENT_NOT_FOUND", "invalid parent id")
		}
		if err := coll.FindOne(ctx, parentFilter).Err(); err != nil {
			return gmod.Errorf("PARENT_NOT_FOUND", "parent group not found")
		}
		// ป้องกันวงจร: ห้าม set parent เป็นลูกหลานของตัวเอง
		descendents, err := collectDescendents(ctx, coll, id)
		if err != nil {
			return err
		}
		for _, d := range descendents {
			if d == *req.ParentID {
				return gmod.Errorf("CIRCULAR_PARENT", "cannot set parent to a descendent")
			}
		}
	}

	now := time.Now().UTC()
	set := bson.M{
		"updatedAt": now,
	}
	unset := bson.M{}

	if req.Name != "" {
		set["name"] = req.Name
	}
	if req.Description != "" {
		set["description"] = req.Description
	}
	// ✅ อัปเดตได้ทั้ง true/false
	if req.Public != nil {
		set["public"] = *req.Public
	}
	if req.Icon != "" {
		set["icon"] = req.Icon
	}
	if req.ParentID != nil {
		// "" หรือ null => ดันเป็น root ด้วย $unset
		if *req.ParentID == "" {
			unset["parentId"] = "" // ค่าอะไรก็ได้ตามคอนเวนชันของ $unset
		} else {
			set["parentId"] = *req.ParentID
		}
	}

	update := bson.M{}
	if len(set) > 0 {
		update["$set"] = set
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}

	// กรณีไม่มีอะไรจะอัปเดต (ไม่น่าเกิดเพราะมี updatedAt เสมอ) ป้องกันไว้
	if len(update) == 0 {
		log.Info().Msg("ℹ️ nothing to update")
		return nil
	}

	_, err = coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	log.Info().
		Str("id", id).
		Interface("update", update).
		Msg("✅ Group updated")
	return nil
}

// idFilter รองรับทั้ง ObjectID และ string _id
func idFilter(id string) (bson.M, error) {
	if oid, err := primitive.ObjectIDFromHex(id); err == nil {
		return bson.M{"_id": oid}, nil
	}
	// สมมุติระบบเก็บ _id เป็น string ได้
	return bson.M{"_id": id}, nil
}

// รวบรวมลูกหลานทั้งหมดของ id (ใช้สำหรับเช็ควงจร)
func collectDescendents(ctx context.Context, coll *mongo.Collection, id string) ([]string, error) {
	type group struct {
		ID       string  `bson:"_id"`
		ParentID *string `bson:"parentId,omitempty"`
	}

	cur, err := coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var all []group
	for cur.Next(ctx) {
		var g group
		if err := cur.Decode(&g); err != nil {
			return nil, err
		}
		all = append(all, g)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	childrenOf := func(pid string) (out []string) {
		for _, g := range all {
			if g.ParentID != nil && *g.ParentID == pid {
				out = append(out, g.ID)
			}
		}
		return
	}

	queue := []string{id}
	seen := map[string]bool{}
	var desc []string

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, c := range childrenOf(n) {
			if !seen[c] {
				seen[c] = true
				desc = append(desc, c)
				queue = append(queue, c)
			}
		}
	}
	return desc, nil
}
