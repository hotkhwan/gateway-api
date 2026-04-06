// controllers/devapi/delete.go
package devapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// DeviceDelete godoc
// @Summary Soft delete a device
// @Description Mark a device as deleted by setting state = "delete"
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /devices/{id} [delete]
// @Security BearerAuth
func DeviceDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.devapi", "DeviceDelete", "devapi", "DeviceDelete")
	defer end()
	id := c.Params("id")
	if id == "" {
		log.Warn().
			Msg("❌ [DeviceDelete] Missing device ID in URL path")
		return httputil.FailBadRequest(c, "Missing device ID")
	}

	log.Info().
		Str("deviceID", id).Msg("🗑️ [DeviceDelete] Attempting soft-delete")

	if err := devsvc.DeviceDelete(ctx, id); err != nil {
		log.Error().
			Err(err).Str("deviceID", id).Msg("❌ [DeviceDelete] Failed to delete device")
		return httputil.FailInternal(c, "Failed to delete device")
	}

	log.Info().
		Str("deviceID", id).Msg("✅ [DeviceDelete] Device soft-deleted successfully")
	return httputil.MessageOK(c, "Device soft-deleted successfully")
}
