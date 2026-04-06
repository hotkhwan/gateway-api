// controllers/devsyncapi/edgeIBOC.go
package devsyncapi

import (
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// EdgeIBOC godoc
// @Summary Sync IBOC devices/channels by Edge ID
// @Tags IBOC
// @Param edgeId path string true "Mongo ObjectId of IBOC Edge"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Security BearerAuth
func EdgeIBOC(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.devsyncapi", "devsync.EdgeIBOC", "devsyncapi", "EdgeIBOC")
	defer end()

	edgeId := c.Params("id")
	if edgeId == "" {
		return httputil.FailBadRequest(c, "id is required")
	}

	log.Debug().Str("edgeId", edgeId).Msg("[EdgeIBOC] sync requested")

	// res, err := devsync.SyncDevicesAndChannelsByEdgeIDIBOC(ctx, edgeId)
	// if err != nil {
	// 	log.Error().Err(err).Str("edgeId", edgeId).Msg("[EdgeIBOC] IBOC sync failed")
	// 	if err == devsync.ErrEdgeNotFound {
	// 		return httputil.FailNotFound(c, err.Error())
	// 	}
	// 	return httputil.FailInternal(c, err.Error())
	// }

	return httputil.MessageOK(c, "IBOC devices/channels synced by edge")
}
