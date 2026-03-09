// controllers/atapi/sync.go
package atapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsync"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// SyncRequest is reserved for future sync options
type SyncRequest struct{}

// DeviceSyncFromSVMS godoc
// @Summary      Sync ATA devices & channels from Edge AI to Klynx
// @Description  Calls ATA Edge AI API to fetch devices and channels then upsert into `camera` collection.
// @Tags         ATA
// @Accept       json
// @Produce      json
// @Param        body body atapi.SyncRequest false "Optional sync options"
// @Success      200 {object} gmod.SuccessDataResponse "Sync result"
// @Failure      502 {object} gmod.ApiErrorResponse "Bad Gateway - ATA upstream error"
// @Failure      500 {object} gmod.ApiErrorResponse "Internal Server Error - config or DB error"
// @Router       /ata/svms/sync [post]
// @Security     BearerAuth
func DeviceSyncFromSVMS(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.atapi", "ata.DeviceSyncFromSVMS", "atapi", "DeviceSyncFromSVMS")
	defer end()

	res, err := devsync.SyncDevicesAndChannelsDefault(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[DeviceSyncFromSVMS] ATA sync failed")
		return httputil.Fail(c, fiber.StatusBadGateway, "ATA_SYNC_FAILED", err.Error())
	}

	return httputil.Ok(c, res, "ATA devices/channels synced")
}
