package grpsvc

import (
	"context"
	"klynx/config"
	"klynx/models/gmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListGroupMembers(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]gmod.Member, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/grpsvc",
		"groups.ListGroupMembers",
		"grpsvc", "ListGroupMembers",
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
			bson.M{"name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"email": bson.M{"$regex": search, "$options": "i"}},
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

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("members")

	var results []gmod.Member
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query member list")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode member list")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count member documents")
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
		Msg("✅ Members listed")

	return results, pagination, nil
}
