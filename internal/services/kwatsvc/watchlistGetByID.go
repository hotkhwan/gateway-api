// internal/services/kwatsvc/getWatchlistByID.go
package kwatsvc

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func WatchlistGetByID(ctx context.Context, id string) (kwatmod.WatchlistResponse, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kwatsvc", // tracerName
		"kwatch.WatchlistGetByID",                 // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kwatsvc", "WatchlistGetByID",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var result kwatmod.WatchlistResponse
	id = strings.TrimSpace(id)

	// base filter (soft delete aware)
	filter := bson.M{
		"$or": []bson.M{
			{"deletedAt": bson.M{"$exists": false}},
			{"deletedAt": nil},
		},
	}
	// ถ้า parse เป็น ObjectID ได้ → _id, ไม่งั้น → idcard
	if objID, err := primitive.ObjectIDFromHex(id); err == nil {
		filter["_id"] = objID
	} else {
		filter["idcard"] = id
	}

	if err := stomongo.FindOne(ctx, "kwatch_watchlist", filter, &result); err != nil {
		return result, err
	}

	// presign photo
	if strings.TrimSpace(result.PhotoKey) != "" {
		bucket := os.Getenv("S3_BUCKET")
		if bucket == "" {
			bucket = os.Getenv("S3_WATCHLIST_BUCKET")
		}
		if bucket != "" {
			if url, perr := stos3minio.PresignOnce(ctx, bucket, result.PhotoKey, 0); perr == nil {
				result.PhotoURL = url
			} else {
				log.Warn().Err(perr).Str("key", result.PhotoKey).Msg("⚠️ presign failed")
			}
		}
	}

	return result, nil
}
