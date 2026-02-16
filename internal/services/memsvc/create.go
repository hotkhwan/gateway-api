package memsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func CreateMember(ctx context.Context, reqs []memmod.MemberRequest) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/memsvc",        // tracerName
		"memsvc.CreateMember", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"memsvc", "CreateMember",
	)
	defer end()

	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	var members []interface{}
	for _, req := range reqs {
		member := bson.M{
			"groupID":  req.GroupID,
			"group":    req.Group,
			"userID":   req.UserID,
			"role":     req.Role,
			"createAt": time.Now(),
		}
		members = append(members, member)
	}

	_, err := collection.InsertMany(ctx, members)
	if err != nil {
		log.Error().Err(err).Msg("❌ Insert member failed")
		return err
	}
	// Successful insert
	log.Info().Int("insert record count", len(reqs)).Msg("✅ Member inserted")
	return nil
}

func CreateMember2(ctx context.Context, groupID string, userIds []string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/memsvc",
		"memsvc.CreateMember2",
		"memsvc", "CreateMember2",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	memberDoc := bson.M{
		"groupID":  groupID,
		"userIds":  userIds,
		"createAt": time.Now(),
	}

	_, err := collection.InsertOne(ctx, memberDoc)
	if err != nil {
		log.Error().Err(err).Msg("❌ Insert member document failed")
		return err
	}

	log.Info().Str("groupID", groupID).Int("userCount", len(userIds)).Msg("✅ Group members array saved")
	return nil
}
