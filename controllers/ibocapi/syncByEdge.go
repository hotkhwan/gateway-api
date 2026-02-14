// controllers/ibocapi/syncByEdge.go
package ibocapi

import (
	"net/http"

	"klynx/internal/services/ibocsvc"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

type IBOCSyncResponse struct {
	Code    string             `json:"code" example:"IBOC_SYNC_OK"`
	Message string             `json:"message" example:"IBOC devices/channels synced"`
	Status  bool               `json:"status" example:"true"`
	Detail  ibocsvc.SyncResult `json:"detail"`
}

// DeviceSyncFromEdgeID godoc
// @Summary Sync IBOC devices/channels by Edge ID
// @Tags IBOC
// @Param edgeId path string true "Mongo ObjectId of IBOC Edge"
// @Success 200 {object} IBOCSyncResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Security BearerAuth
func DeviceSyncFromEdgeID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/ibocapi")
	ctx, span := tracer.Start(ctx, "IBOC.DeviceSyncFromEdgeID")
	defer span.End()

	edgeId := c.Params("edgeId")
	if edgeId == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "edgeId is required",
			Status:  false,
		})
	}

	res, err := ibocsvc.SyncDevicesAndChannelsByEdgeID(ctx, edgeId)
	if err != nil {
		code := http.StatusInternalServerError
		if err == ibocsvc.ErrEdgeNotFound {
			code = http.StatusNotFound
		}
		return c.Status(code).JSON(gmod.ErrorMessageResponse{
			Code:    "IBOC_SYNC_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(IBOCSyncResponse{
		Code:    "IBOC_SYNC_OK",
		Message: "IBOC devices/channels synced by edge",
		Status:  true,
		Detail:  *res,
	})
}
