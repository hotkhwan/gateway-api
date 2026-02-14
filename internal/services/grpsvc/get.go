// internal/services/grpsvc/get.go
package grpsvc

import (
	"context"
	"klynx/config"
	"klynx/models/gmod"
	"klynx/models/grpmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListGroups(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]grpmod.Group, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/grpsvc",
		"grpsvc.ListGroups",
		"grpsvc", "ListGroups",
	)
	defer end()

	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	filter := bson.M{}

	// ✅ Support select=subgroups + filter=<parentId>
	if filters["select"] == "subgroups" {
		if parentId := filters["filter"]; parentId != "" {
			filter["parentId"] = parentId
		}
	}

	// ✅ Support search by name or _id
	if search := filters["search"]; search != "" {
		searchOr := bson.A{
			bson.M{"name": bson.M{"$regex": search, "$options": "i"}},
		}
		// If valid ObjectID
		if objID, err := primitive.ObjectIDFromHex(search); err == nil {
			searchOr = append(searchOr, bson.M{"_id": objID})
		}
		filter["$or"] = searchOr
	}

	// Pagination & Sort
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
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query group list")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode groups")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count group documents")
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
		Interface("mongoFilter", filter).
		Msg("✅ Groups listed")

	return results, pagination, nil
}
func BuildGroupTree(ctx context.Context, groups []grpmod.Group, parentId *string) []grpmod.GroupTree {
	var tree []grpmod.GroupTree
	for _, g := range groups {
		if (g.ParentID == nil && parentId == nil) || (g.ParentID != nil && parentId != nil && *g.ParentID == *parentId) {
			node := grpmod.GroupTree{
				ID:          g.ID,
				Name:        g.Name,
				Public:      g.Public,
				Description: g.Description,
				Members:     g.Members, // << เพิ่มตรงนี้
				Children:    BuildGroupTree(ctx, groups, &g.ID),
			}
			tree = append(tree, node)
		}
	}
	return tree
}

func GetAllGroups(ctx context.Context) ([]grpmod.Group, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/grpsvc",        // tracerName
		"grpsvc.GetAllGroups", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"grpsvc", "GetAllGroups",
	)
	defer end()

	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("groups")

	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query all groups")
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []grpmod.Group
	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode groups")
		return nil, err
	}

	// ✅ Normalize empty parentId string to nil
	for i := range results {
		if results[i].ParentID != nil && *results[i].ParentID == "" {
			results[i].ParentID = nil
		}
	}

	return results, nil
}
