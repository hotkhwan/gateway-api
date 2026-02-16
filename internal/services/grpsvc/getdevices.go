// internal/services/grpsvc/getdevices.go
package grpsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListGroupDevices(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]devmod.Device, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/grpsvc",
		"groups.ListGroupDevices",
		"grpsvc", "ListGroupDevices",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter := bson.M{}

	// ✅ filter = groupId
	if groupId := filters["filter"]; groupId != "" {
		filter["groupId"] = groupId
	}

	// ✅ search by name or _id
	if search := filters["search"]; search != "" {
		searchOr := bson.A{
			bson.M{"name": bson.M{"$regex": search, "$options": "i"}},
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

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("devices")

	var results []devmod.Device
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query device list")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode device list")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count device documents")
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
		Msg("✅ Devices listed")

	return results, pagination, nil
}
