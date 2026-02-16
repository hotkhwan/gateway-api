package memsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListMembers(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]memmod.Member, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/memsvc",
		"memsvc.ListMembers",
		"memsvc", "ListMembers",
	)
	defer end()

	// Timeout after starting the span to preserve trace context
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📥 Listing devices")

	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	if sortField == "" {
		sortField = "CreatedAt"
	}

	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}})

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	var results []memmod.MemberMongo

	cursor, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to query group members")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode group members")
		return nil, gmod.Pagination{}, err
	}

	members := make([]memmod.Member, 0, len(results))
	for _, d := range results {
		members = append(members, memmod.Member{
			ID:        d.ID.Hex(),
			GroupID:   d.GroupID,
			Group:     d.Group,
			UserID:    d.UserID,
			Role:      d.Role,
			CreatedAt: utils.FormatTimeOrEmpty(d.CreatedAt),
		})
	}

	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: len(members),
		TotalPages:   (len(members) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	log.Debug().
		Int("count", len(members)).
		Msg("✅ Group members listed")

	return members, pagination, nil

}
