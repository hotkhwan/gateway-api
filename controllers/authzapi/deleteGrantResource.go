// controllers/authzapi/deleteGrantResource.go
package authzapi

import (
	"klynx/internal/services/authzsvc"
	"klynx/models/authzmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// RevokeResourceHandler godoc
// @Summary      Revoke grant permission relationships to resource
// @Description  Revoke grant owner, editor, viewer, assigned group/role, and attributes (type, crud) from a resource entity
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        request body authzmod.RevokeResourceRequest true "Revoke grant resource permission payload"
// @Success      200 {object} gmod.SuccessMessageResponse "Resource revoke granted successfully"
// @Failure      400 {object} gmod.ErrorMessageResponse "Invalid input or missing fields"
// @Failure      500 {object} gmod.ErrorMessageResponse "Internal error from Permify"
// @Router       /authz/resource/revoke [post]
// @Security     BearerAuth
func RevokeResourceHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.RevokeResourceHandler")
	defer span.End()

	var req authzmod.RevokeResourceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}

	if req.EntityType == "" || len(req.EntityIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "entityType and entityIds are required",
			Status:  false,
		})
	}

	if err := authzsvc.RevokeResource(ctx, req.EntityType, req.EntityIDs); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "✅ Entities revoked successfully",
		Status:  true,
	})
}
