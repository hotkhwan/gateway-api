package memsvc

import (
	"context"
	"errors"
	"fmt"
	"klynx/config"
	"klynx/models/memmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func MemberGetByID(ctx context.Context, id string) (*memmod.Member, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/memsvc",
		"memsvc.MemberGetByID",
		"memsvc", "MemberGetByID",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("group_members")

	// ✅ บังคับใช้ ObjectID เท่านั้น
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Invalid ObjectID format")
		return nil, fmt.Errorf("invalid ObjectID: %w", err)
	}

	filter := bson.M{"_id": oid}
	opts := options.FindOne()

	log.Debug().Interface("filter", filter).Msg("🔎 Get member by id")

	var raw memmod.Member
	if err := coll.FindOne(ctx, filter, opts).Decode(&raw); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			log.Warn().Str("id", id).Msg("⚠️ Member not found or deleted")
			return nil, mongo.ErrNoDocuments
		}
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to get member by ID")
		return nil, err
	}

	log.Debug().Str("id", raw.ID).Msg("✅ MemberGetByID success")
	return &raw, nil
}
