// internal/services/kctrlsvc/deviceById.go
package kctrlsvc

import (
	"context"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Get device by ObjectID (hex) หรือ ถ้าไม่ใช่ hex จะลองแมตช์กับ hwId / code
func DeviceGetByID(ctx context.Context, id string) (kctrlmod.Device, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kctrlsvc",
		"kctrlsvc.DeviceGetByID",
		"kctrlsvc", "DeviceGetByID",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var result kctrlmod.Device
	id = strings.TrimSpace(id)

	// base filter (soft delete aware)
	filter := bson.M{
		"$or": []bson.M{
			{"deletedAt": bson.M{"$exists": false}},
			{"deletedAt": nil},
		},
	}

	// ระบุเงื่อนไขค้นหา
	if objID, err := primitive.ObjectIDFromHex(id); err == nil {
		filter["_id"] = objID
	} else {
		// ถ้าไม่ใช่ ObjectID: ลองหา hwId หรือ code
		filter["$and"] = []bson.M{
			{
				"$or": []bson.M{
					{"hwId": id},
					{"code": id},
				},
			},
		}
	}

	log.Debug().
		Str("id", id).
		Interface("filter", filter).
		Msg("🔎 [DeviceGetByID] query device")

	if err := stomongo.FindOne(ctx, "kcontrol", filter, &result); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ [DeviceGetByID] not found")
		return result, err
	}

	log.Debug().
		Str("id", id).
		Str("name", result.Name).
		Str("hwId", result.HwID).
		Msg("✅ [DeviceGetByID] found")

	return result, nil
}
