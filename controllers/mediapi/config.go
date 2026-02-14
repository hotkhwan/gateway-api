// controllers/mediapi/config.go
package mediapi

import (
	"klynx/config"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	ctx := c.UserContext()

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
	}

	out := map[string]any{
		"streamSessionTimeout": ttl,
	}

	return c.JSON(gmod.SuccessDetailResponseWithMSG{
		Code:    "SUCCESS",
		Status:  true,
		Message: "OK",
		Detail:  out,
	})
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
	ctx := c.UserContext()
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
			return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
		}

		if body.MapLocation.Lat == "" || body.MapLocation.Lng == "" {
			return httputil.FailBadRequest(c, "INVALID_BODY", "mapLocation.lat and mapLocation.lng are required")
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
			return httputil.FailInternalReason(c, "INTERNAL_ERROR", "DB_ERROR")
		}

		return c.JSON(gmod.SuccessMessageResponse{
			Code:    "SUCCESS",
			Message: "updated",
			Status:  true,
		})

	case "system.zoomLevel":
		var body struct {
			ZoomLevel int `json:"zoomLevel"`
		}

		if err := c.BodyParser(&body); err != nil {
			return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
		}

		if body.ZoomLevel < 0 {
			return httputil.FailBadRequest(c, "INVALID_BODY", "zoomLevel must be >= 0")
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
			return httputil.FailInternalReason(c, "INTERNAL_ERROR", "DB_ERROR")
		}

		return c.JSON(gmod.SuccessMessageResponse{
			Code:    "SUCCESS",
			Message: "updated",
			Status:  true,
		})

	default:
		// เดิม: streamSessionTimeout
		var body struct {
			StreamSessionTimeout int `json:"streamSessionTimeout"`
		}

		if err := c.BodyParser(&body); err != nil {
			return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
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
			return httputil.FailInternalReason(c, "INTERNAL_ERROR", "DB_ERROR")
		}

		return c.JSON(gmod.SuccessMessageResponse{
			Code:    "SUCCESS",
			Message: "updated",
			Status:  true,
		})
	}
}
