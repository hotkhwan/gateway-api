// controllers/devsyncapi/edgeATA.go
package devsyncapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsync"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// EdgeATA godoc
// @Summary Sync ATA devices/channels by Edge ID
// @Tags ATA
// @Param edgeId path string true "Mongo ObjectId of ATA Edge"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Security BearerAuth
func EdgeATA(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.devsyncapi", "devsync.EdgeATA", "devsyncapi", "EdgeATA")
	defer end()

	edgeId := c.Params("id")
	if edgeId == "" {
		return httputil.FailBadRequest(c, "id is required")
	}

	res, err := devsync.SyncDevicesAndChannelsByEdgeIDATA(ctx, edgeId)
	if err != nil {
		log.Error().Err(err).Str("edgeId", edgeId).Msg("[EdgeATA] ATA sync failed")
		if err == devsync.ErrEdgeNotFound {
			return httputil.FailNotFound(c, err.Error())
		}
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "ATA devices/channels synced by edge")
}
