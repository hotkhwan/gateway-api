package memsvc

import (
	"context"
	"klynx/config"
	"klynx/models/memmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UpdateMember(ctx context.Context, memberID string, req memmod.MemberRequest) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/memsvc",
		"memsvc.UpdateMember",
		"memsvc", "UpdateMember",
	)
	defer end()

	// timeout
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	objID, err := primitive.ObjectIDFromHex(memberID)
	if err != nil {
		log.Error().Err(err).Str("memberID", memberID).Msg("❌ Invalid member ID")
		return err
	}

	filter := bson.M{"_id": objID}

	_, err = collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("memberID", memberID).Msg("❌ Delete member failed")
		return err
	}

	member := bson.M{
		"_id":      objID,
		"GroupID":  req.GroupID,
		"Group":    req.Group,
		"UserID":   req.UserID,
		"Role":     req.Role,
		"joinedAt": time.Now(),
	}

	_, err = collection.InsertOne(ctx, member)
	if err != nil {
		log.Error().Err(err).Str("memberID", memberID).Msg("❌ Insert member failed (after delete)")
		return err
	}

	log.Info().
		Str("memberID", memberID).
		Str("userID", req.UserID).
		Str("groupID", req.GroupID).
		Msg("✅ Member updated (delete → create)")

	return nil
}
