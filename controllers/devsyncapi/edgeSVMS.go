// controllers/devsyncapi/edgeSVMS.go
package devsyncapi

import (
	"net/http"

	"klynx/internal/services/devsync"
	"klynx/models/devsyncmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// EdgeSVMS godoc
// @Summary Sync SVMS devices/channels by Edge ID
// @Tags SVMS
// @Param id path string true "Mongo ObjectId of SVMS Edge"
// @Success 200 {object} devsyncmod.SVMSSyncResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Security BearerAuth
func EdgeSVMS(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/devsyncapi")
	ctx, span := tracer.Start(ctx, "SVMS.EdgeSVMS")
	defer span.End()

	edgeId := c.Params("id")
	if edgeId == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "id is required",
			Status:  false,
		})
	}

	res, err := devsync.SyncDevicesAndChannelsByEdgeIDSVMS(ctx, edgeId)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == devsync.ErrEdgeNotFound {
			statusCode = http.StatusNotFound
		}

		return c.Status(statusCode).JSON(gmod.ErrorMessageResponse{
			Code:    "SVMS_SYNC_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(devsyncmod.SVMSSyncResponse{
		Code:    "SVMS_SYNC_OK",
		Message: "SVMS devices/channels synced by edge",
		Status:  true,
		Detail:  *res,
	})
}
