// controllers/mediapi/config.go
package mediapi

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ListConfig godoc
// @Summary      List stream session configs
// @Description  Get stream session timeout configs
// @Tags         Media
// @Produce      json
// @Success      200 {object} gmod.Response
// @Failure      500 {object} gmod.ErrorResponse
// @Router       /system/stream [get]
// @Security     BearerAuth
func ListConfig(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.mediapi", "ListConfig", "mediapi", "ListConfig")
	defer end()

	var opt bson.M
	err := config.DB.
		Collection("options").
		FindOne(ctx, bson.M{"_id": "system.setting"}).
		Decode(&opt)

	// ✅ default = 1 hour
	var ttl int64 = 3600

	if err == nil {
		if v, ok := opt["streamSessionTimeout"]; ok {
			switch n := v.(type) {
			case int64:
				if n > 0 {
					ttl = n
				}
			case int32:
				if n > 0 {
					ttl = int64(n)
				}
			case int:
				if n > 0 {
					ttl = int64(n)
				}
			case float64:
				if n > 0 {
					ttl = int64(n)
				}
			}
		}
	} else {
		log.Warn().Err(err).Msg("failed to read system.setting, using default TTL")
	}

	out := map[string]any{
		"streamSessionTimeout": ttl,
	}

	return httputil.Ok(c, out)
}

// UpdateConfig godoc
// @Summary      Update stream config
// @Tags         Media
// @Accept       json
// @Produce      json
// @Param        id   path      string true "Config ID"
// @Param        body body      object true "Update payload"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      400 {object} gmod.BadRequestResponse
// @Failure      500 {object} gmod.ErrorResponse
// @Router       /system/stream/{id} [patch]
// @Security     BearerAuth
func UpdateConfig(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.mediapi", "UpdateConfig", "mediapi", "UpdateConfig")
	defer end()

	id := c.Params("id")

	switch id {

	case "system.mapLocation":
		var body struct {
			MapLocation struct {
				Lat string `json:"lat"`
				Lng string `json:"lng"`
			} `json:"mapLocation"`
		}

		if err := c.BodyParser(&body); err != nil {
			return httputil.FailBadRequest(c, err.Error())
		}

		if body.MapLocation.Lat == "" || body.MapLocation.Lng == "" {
			return httputil.FailBadRequest(c, "mapLocation.lat and mapLocation.lng are required")
		}

		set := bson.M{
			"mapLocation": bson.M{
				"lat": body.MapLocation.Lat,
				"lng": body.MapLocation.Lng,
			},
			"updatedAt": time.Now(),
		}

		_, err := config.DB.Collection("options").UpdateByID(
			ctx,
			"system.setting",
			bson.M{"$set": set},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			log.Error().Err(err).Msg("DB update failed for mapLocation")
			return httputil.FailInternal(c, "database error")
		}

		return httputil.MessageOK(c, "updated")

	case "system.zoomLevel":
		var body struct {
			ZoomLevel int `json:"zoomLevel"`
		}

		if err := c.BodyParser(&body); err != nil {
			return httputil.FailBadRequest(c, err.Error())
		}

		if body.ZoomLevel < 0 {
			return httputil.FailBadRequest(c, "zoomLevel must be >= 0")
		}

		set := bson.M{
			"zoomLevel": body.ZoomLevel,
			"updatedAt": time.Now(),
		}

		_, err := config.DB.Collection("options").UpdateByID(
			ctx,
			"system.setting",
			bson.M{"$set": set},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			log.Error().Err(err).Msg("DB update failed for zoomLevel")
			return httputil.FailInternal(c, "database error")
		}

		return httputil.MessageOK(c, "updated")

	default:
		// เดิม: streamSessionTimeout
		var body struct {
			StreamSessionTimeout int `json:"streamSessionTimeout"`
		}

		if err := c.BodyParser(&body); err != nil {
			return httputil.FailBadRequest(c, err.Error())
		}

		set := bson.M{
			"streamSessionTimeout": body.StreamSessionTimeout,
			"updatedAt":            time.Now(),
		}

		_, err := config.DB.Collection("options").UpdateByID(
			ctx,
			"system.setting",
			bson.M{"$set": set},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			log.Error().Err(err).Msg("DB update failed for streamSessionTimeout")
			return httputil.FailInternal(c, "database error")
		}

		return httputil.MessageOK(c, "updated")
	}
}
