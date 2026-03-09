// controllers/devapi/update.go
package devapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeviceUpdate godoc
// @Summary Update a device
// @Description Update fields of a device and set state to "update"
// @Tags Devices
// @Accept json
// @Produce json
// @Param id path string true "Device ID"
// @Param device body devmod.Device true "Device object"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /devices/{id} [patch]
// @Security BearerAuth
func DeviceUpdate(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.devapi", "DeviceUpdate", "devapi", "DeviceUpdate")
	defer end()
	id := c.Params("id")
	if id == "" {
		log.Warn().
			Msg("❌ [DeviceUpdate] Missing device ID")
		return httputil.FailBadRequest(c, "Missing device ID")
	}

	var req devmod.Device
	if err := c.BodyParser(&req); err != nil {
		log.Warn().
			Str("deviceID", id).Msg("❌ [DeviceUpdate] Invalid request body")
		return httputil.FailBadRequest(c, "Invalid request body")
	}

	log.Info().
		Str("deviceID", id).
		Str("name", req.Name).
		Str("district", req.District).
		Str("brand", req.Brand).
		Msg("✏️ [DeviceUpdate] Attempting update")

	if err := devsvc.DeviceUpdate(ctx, id, req); err != nil {
		log.Error().
			Err(err).Str("deviceID", id).Msg("❌ [DeviceUpdate] Failed to update device")
		return httputil.FailInternal(c, "Failed to update device")
	}

	log.Info().
		Str("deviceID", id).Msg("✅ [DeviceUpdate] Device updated successfully")
	return httputil.MessageOK(c, "Device updated successfully")
}
