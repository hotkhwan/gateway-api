// internal/services/grpsvc/getsubgroup.go
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
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ListSubGroups คืนรายการกลุ่มที่มี parentId = filters["filter"]
func ListSubGroups(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]grpmod.Group, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/grpsvc",
		"grpsvc.ListSubGroups",
		"grpsvc", "ListSubGroups",
	)
	defer end()

	// timeout
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	filter := bson.M{}

	// ✅ บังคับใช้ parentId จาก filters["filter"]
	if pid := filters["filter"]; pid != "" {
		// เผื่อ schema เก็บ parentId เป็น string หรือ ObjectID ไม่แน่ใจ — รองรับทั้งสองแบบ
		or := bson.A{bson.M{"parentId": pid}}
		if objID, err := primitive.ObjectIDFromHex(pid); err == nil {
			or = append(or, bson.M{"parentId": objID})
		}
		filter["$or"] = or
	}

	// ✅ ค้นหาด้วยชื่อหรือ _id (ซ้อน $and ถ้ามีเงื่อนไขอื่นแล้ว)
	if s := filters["search"]; s != "" {
		searchOr := bson.A{
			bson.M{"name": bson.M{"$regex": s, "$options": "i"}},
		}
		if objID, err := primitive.ObjectIDFromHex(s); err == nil {
			searchOr = append(searchOr, bson.M{"_id": objID})
		}
		if len(filter) == 0 {
			filter = bson.M{"$or": searchOr}
		} else {
			filter = bson.M{
				"$and": bson.A{
					filter,
					bson.M{"$or": searchOr},
				},
			}
		}
	}

	// Pagination & Sort
	if page < 1 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}
	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}})

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("groups")

	var results []grpmod.Group
	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Interface("filter", filter).Msg("❌ Failed to query subgroup list")
		return nil, gmod.Pagination{}, err
	}
	defer cur.Close(ctx)

	if err := cur.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode subgroups")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count subgroup documents")
		return nil, gmod.Pagination{}, err
	}

	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	log.Debug().
		Int("resultCount", len(results)).
		Int("total", int(total)).
		Interface("mongoFilter", filter).
		Msg("✅ Subgroups listed")

	return results, pagination, nil
}
