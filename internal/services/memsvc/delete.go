package memsvc

import (
	"context"
	"klynx/config"

	//"klynx/models/memmod"
	"klynx/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func DeleteMember(ctx context.Context, memberId string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/memsvc",        // tracerName
		"memsvc.DeleteMember", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"memsvc", "DeleteMember",
	)
	defer end()

	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("group_members")

	objID, err := primitive.ObjectIDFromHex(memberId)
	if err != nil {
		log.Error().Err(err).Str("memberID", memberId).Msg("❌ Invalid member ID")
		return err
	}

	filter := bson.M{"_id": objID}

	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Error().Err(err).Str("memberID", memberId).Msg("❌ Delete member failed")
		return err
	}

	if result.DeletedCount == 0 {
		log.Warn().Str("memberID", memberId).Msg("⚠️ No member found to delete")
		return mongo.ErrNoDocuments
	}

	log.Info().Str("memberID", memberId).Msg("✅ Member deleted")
	return nil
}
