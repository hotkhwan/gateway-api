// internal/services/systemsvc/edgesvc/softDelete.go
package edgesvc

import (
	"context"
	"errors"
	"strings"

	"klynx/internal/logger"
	"klynx/internal/repo/stomongo"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func SoftDeleteEdge(ctx context.Context, id string, deletedBy string) error {
	log := logger.FromCtx(ctx, "edgesvc", "SoftDeleteEdge")

	oid, err := primitive.ObjectIDFromHex(strings.TrimSpace(id))
	if err != nil {
		return errors.New("invalid id")
	}

	// ✅ ไม่ลบซ้ำ
	filter := bson.M{"_id": oid, "isDeleted": bson.M{"$ne": true}}

	_, err = stomongo.SoftDeleteMany(ctx, systemEdgeColl, filter, stomongo.SoftDeleteOptions{
		State: "archived",
		ExtraSet: bson.M{
			"deletedBy": strings.TrimSpace(deletedBy),
		},
	})
	if err != nil {
		log.Error().Err(err).Str("id", oid.Hex()).Msg("soft delete edge failed")
		return err
	}

	log.Info().Str("id", oid.Hex()).Msg("edge soft-deleted")
	return nil
}
