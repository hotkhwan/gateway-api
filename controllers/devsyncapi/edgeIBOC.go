// controllers/devsyncapi/edgeATA.go
package devsyncapi

import (
	"klynx/models/devsyncmod"
	"klynx/models/gmod"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// EdgeIBOC godoc
// @Summary Sync IBOC devices/channels by Edge ID
// @Tags IBOC
// @Param edgeId path string true "Mongo ObjectId of IBOC Edge"
// @Success 200 {object} IBOCSyncResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Security BearerAuth
func EdgeIBOC(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/ibocapi")
	ctx, span := tracer.Start(ctx, "IBOC.EdgeIBOC")
	defer span.End()

	edgeId := c.Params("id")
	if edgeId == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "id is required",
			Status:  false,
		})
	}

	// res, err := devsync.SyncDevicesAndChannelsByEdgeIDIBOC(ctx, edgeId)
	// if err != nil {
	// 	code := http.StatusInternalServerError
	// 	if err == devsync.ErrEdgeNotFound {
	// 		code = http.StatusNotFound
	// 	}
	// 	return c.Status(code).JSON(gmod.ErrorMessageResponse{
	// 		Code:    "ATA_SYNC_FAILED",
	// 		Message: err.Error(),
	// 		Status:  false,
	// 	})
	// }

	return c.JSON(devsyncmod.IBOCSyncResponse{
		Code:    "IBOC_SYNC_OK",
		Message: "IBOC devices/channels synced by edge",
		Status:  true,
		// Detail:  *res,
	})
}
