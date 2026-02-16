// internal/gateways/watchman/watchgw/watchmanUpdate.go
package watchgw

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const watchlistsColl = "watchlists"

// อัปเดต watchmanId ตาม _id ที่ระบุ
// คืนค่า matched, modified, error
func SaveWatchmanID(ctx context.Context, watchlistID primitive.ObjectID, watchmanID int64) (int64, int64, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/watchgw",
		"watchman.SaveWatchmanID",
		"watchgw", "SaveWatchmanID",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if watchlistID == primitive.NilObjectID {
		return 0, 0, errors.New("invalid watchlistID (nil ObjectID)")
	}

	// กันค้างสั้น ๆ (ยังเคารพ deadline จาก ctx เดิม)
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := stomongo.UpdateByID(opCtx, watchlistsColl, watchlistID, bson.M{
		"watchmanId": watchmanID, // updatedAt จะถูกเติมโดย nowSet() ใน stomongo ให้อัตโนมัติ
	})
	if err != nil {
		log.Error().Str("watchlistID", watchlistID.Hex()).Int64("watchmanID", watchmanID).Err(err).
			Msg("❌ UpdateByID failed")
		return 0, 0, err
	}

	log.Debug().
		Str("watchlistID", watchlistID.Hex()).
		Int64("watchmanID", watchmanID).
		Int64("matched", res.MatchedCount).
		Int64("modified", res.ModifiedCount).
		Msg("✅ watchmanId updated")
	return res.MatchedCount, res.ModifiedCount, nil
}
