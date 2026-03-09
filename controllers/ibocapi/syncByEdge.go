// controllers/ibocapi/syncByEdge.go
package ibocapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/ibocsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeviceSyncFromEdgeID godoc
// @Summary Sync IBOC devices/channels by Edge ID
// @Tags IBOC
// @Param edgeId path string true "Mongo ObjectId of IBOC Edge"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Security BearerAuth
func DeviceSyncFromEdgeID(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ibocapi", "iboc.DeviceSyncFromEdgeID", "ibocapi", "DeviceSyncFromEdgeID")
	defer end()

	edgeId := c.Params("edgeId")
	if edgeId == "" {
		return httputil.FailBadRequest(c, "edgeId is required")
	}

	res, err := ibocsvc.SyncDevicesAndChannelsByEdgeID(ctx, edgeId)
	if err != nil {
		log.Error().Err(err).Str("edgeId", edgeId).Msg("[DeviceSyncFromEdgeID] IBOC sync failed")
		if err == ibocsvc.ErrEdgeNotFound {
			return httputil.FailNotFound(c, err.Error())
		}
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "IBOC devices/channels synced by edge")
}
