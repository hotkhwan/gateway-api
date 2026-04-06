// controllers/devsyncapi/edgeSVMS.go
package devsyncapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsync"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// EdgeSVMS godoc
// @Summary Sync SVMS devices/channels by Edge ID
// @Tags SVMS
// @Param id path string true "Mongo ObjectId of SVMS Edge"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Security BearerAuth
func EdgeSVMS(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.devsyncapi", "devsync.EdgeSVMS", "devsyncapi", "EdgeSVMS")
	defer end()

	edgeId := c.Params("id")
	if edgeId == "" {
		return httputil.FailBadRequest(c, "id is required")
	}

	res, err := devsync.SyncDevicesAndChannelsByEdgeIDSVMS(ctx, edgeId)
	if err != nil {
		log.Error().Err(err).Str("edgeId", edgeId).Msg("[EdgeSVMS] SVMS sync failed")
		if err == devsync.ErrEdgeNotFound {
			return httputil.FailNotFound(c, err.Error())
		}
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "SVMS devices/channels synced by edge")
}
