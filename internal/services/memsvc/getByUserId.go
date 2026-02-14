package memsvc

import (
	"context"
	"klynx/config"
	"klynx/models/memmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func MembersGetByUserID(ctx context.Context, userID string) ([]memmod.Member, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/memsvc",
		"memsvc.MembersGetByUserID",
		"memsvc", "MembersGetByUserID",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	filter := bson.M{"userID": userID}

	log.Debug().Interface("filter", filter).Msg("🔎 Get members by UserID")

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("UserID", userID).Msg("❌ Failed to find members by UserID")
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []memmod.Member
	if err := cursor.All(ctx, &members); err != nil {
		log.Error().Err(err).Str("UserID", userID).Msg("❌ Failed to decode members list")
		return nil, err
	}

	if len(members) == 0 {
		log.Warn().Str("UserID", userID).Msg("⚠️ No members found for UserID")
		return nil, mongo.ErrNoDocuments
	}

	return members, nil
}

func GetGroupByUserID(ctx context.Context, userID string) ([]string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/memsvc",
		"memsvc.GetGroupByUserID",
		"memsvc", "GetGroupByUserID",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	// เปลี่ยน Filter: หา userID ที่ปรากฏอยู่ใน Array "userIds"
	filter := bson.M{"userIds": userID}

	log.Debug().Interface("filter", filter).Msg("🔎 Get groupIDs by UserID")

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []memmod.Member
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}

	if len(members) == 0 {
		return nil, nil
	}

	// สกัดเอาเฉพาะ groupID ออกมาใส่ใน String Array
	groupIDs := make([]string, 0, len(members))
	for _, m := range members {
		if m.GroupID != "" {
			groupIDs = append(groupIDs, m.GroupID)
		}
	}

	return groupIDs, nil
}
