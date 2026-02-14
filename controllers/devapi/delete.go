// controllers/devapi/delete.go
package devapi

import (
	"klynx/internal/services/devsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
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
func DeviceDelete(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/devapi", "devices.DeviceDelete", "devapi", "DeviceDelete")
	defer span.End()
	id := c.Params("id")
	if id == "" {
		log.Warn().
			Msg("❌ [DeviceDelete] Missing device ID in URL path")
		return httputil.FailBadRequest(c, "MISSING_ID", "Missing device ID")
	}

	log.Info().
		Str("deviceID", id).Msg("🗑️ [DeviceDelete] Attempting soft-delete")

	if err := devsvc.DeviceDelete(ctx, id); err != nil {
		log.Error().
			Err(err).Str("deviceID", id).Msg("❌ [DeviceDelete] Failed to delete device")
		return httputil.FailInternal(c, "DELETE_FAILED", err.Error())
	}

	log.Info().
		Str("deviceID", id).Msg("✅ [DeviceDelete] Device soft-deleted successfully")
	return gmod.SendMessageOK(c, "DEVICE_DELETE_SUCCESS", "Device soft-deleted")
}
