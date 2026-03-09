// controllers/sysapi/setapi/maplocation.go
package setapi

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/setsvc"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.sysapi", "SetApi.ListConfig", "sysapi", "ListConfig")
	defer end()

	setting, err := setsvc.GetMapSetting(ctx, config.DB)
	if err != nil {
		log.Error().Err(err).Msg("get map setting failed")
		return httputil.FailInternalReason(c, "internal server error", "DB_ERROR")
	}

	out := fiber.Map{
		"mapLocation": setting.MapLocation,
		"zoomLevel":   setting.ZoomLevel,
	}

	return httputil.Ok(c, out)
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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.sysapi", "SetApi.UpdateConfig", "sysapi", "UpdateConfig")
	defer end()

	id := c.Params("id")

	var body usrmod.MapSetting
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}

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
		log.Error().Err(err).Msg("update map setting failed")
		return httputil.FailInternalReason(c, "internal server error", "DB_ERROR")
	}

	return httputil.MessageOK(c, "updated")
}
