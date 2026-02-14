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

func ListGroupRoles(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]grpmod.GroupRole, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/grpsvc",
		"groups.ListGroupRoles",
		"grpsvc", "ListGroupRoles",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{}

	if groupId := filters["filter"]; groupId != "" {
		filter["groupId"] = groupId
	}

	if search := filters["search"]; search != "" {
		searchOr := bson.A{
			bson.M{"role": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"description": bson.M{"$regex": search, "$options": "i"}},
		}
		if objID, err := primitive.ObjectIDFromHex(search); err == nil {
			searchOr = append(searchOr, bson.M{"_id": objID})
		}
		filter["$or"] = searchOr
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

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("group_roles")

	var results []grpmod.GroupRole
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query group_roles")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode group_roles")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count group_roles")
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
		Msg("✅ Roles listed")

	return results, pagination, nil
}
