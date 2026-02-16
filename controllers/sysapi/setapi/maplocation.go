// controllers/sysapi/setapi/maplocation.go
package setapi

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/setsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"

	"github.com/gofiber/fiber/v2"
)

// ListConfig godoc
// @Summary      Get map location center
// @Description  Get map center lat/lng + zoomLevel from system setting
// @Tags         System
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponseWithMSG
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /system/setting/mapLocation [get]
// @Security     BearerAuth
func ListConfig(c *fiber.Ctx) error {
	ctx := c.UserContext()

	setting, err := setsvc.GetMapSetting(ctx, config.DB)
	if err != nil {
		// normalize: code=INTERNAL_ERROR, reason=DB_ERROR
		return httputil.FailInternalReason(c, "INTERNAL_ERROR", "db error")
	}

	out := map[string]any{
		"mapLocation": setting.MapLocation,
		"zoomLevel":   setting.ZoomLevel,
	}

	return c.JSON(gmod.SuccessDetailResponseWithMSG{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "OK",
		Detail:  out,
	})
}

// UpdateConfig godoc
// @Summary      Update map location center
// @Tags         System
// @Accept       json
// @Produce      json
// @Param        id   path      string true "Config ID"
// @Param        body body      usrmod.MapSetting true "Update payload"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /system/setting/mapLocation/{id} [patch]
// @Security     BearerAuth
func UpdateConfig(c *fiber.Ctx) error {
	ctx := c.UserContext()
	id := c.Params("id")

	var body usrmod.MapSetting
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequestReason(c, "INVALID_BODY", "invalid request body", nil)
	}

	// validate แบบไม่ปล่อยผ่านเงียบๆ
	if body.MapLocation.Lat == "" || body.MapLocation.Lng == "" {
		return httputil.FailBadRequestReason(
			c,
			"mapLocation.lat and mapLocation.lng are required",
			"INVALID_BODY",
			httputil.ErrorDetails{
				"fields": map[string]string{
					"mapLocation.lat": "required",
					"mapLocation.lng": "required",
				},
			},
		)
	}

	if body.ZoomLevel < 0 {
		return httputil.FailBadRequestReason(
			c,
			"zoomLevel must be >= 0",
			"INVALID_BODY",
			httputil.ErrorDetails{
				"fields": map[string]string{
					"zoomLevel": "must be >= 0",
				},
			},
		)
	}

	if err := setsvc.UpdateMapSetting(ctx, config.DB, id, body); err != nil {
		return httputil.FailInternalReason(c, "internal server error", "DB_ERROR")
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "updated",
		Status:  true,
	})
}
