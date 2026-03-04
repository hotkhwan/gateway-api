// controllers/devsyncapi/edgeATA.go
package devsyncapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsync"
	"github.com/hotkhwan/gateway-api/models/devsyncmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// EdgeATA godoc
// @Summary Sync ATA devices/channels by Edge ID
// @Tags ATA
// @Param edgeId path string true "Mongo ObjectId of ATA Edge"
// @Success 200 {object} devsyncmod.ATASyncResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Security BearerAuth
func EdgeATA(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/atapi")
	ctx, span := tracer.Start(ctx, "ATA.EdgeATA")
	defer span.End()

	edgeId := c.Params("id")
	if edgeId == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "id is required",
			Status:  false,
		})
	}

	res, err := devsync.SyncDevicesAndChannelsByEdgeIDATA(ctx, edgeId)
	if err != nil {
		code := http.StatusInternalServerError
		if err == devsync.ErrEdgeNotFound {
			code = http.StatusNotFound
		}
		return c.Status(code).JSON(gmod.ErrorMessageResponse{
			Code:    "ATA_SYNC_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(devsyncmod.ATASyncResponse{
		Code:    "ATA_SYNC_OK",
		Message: "ATA devices/channels synced by edge",
		Status:  true,
		Detail:  *res,
	})
}
