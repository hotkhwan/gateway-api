// controllers/authzapi/postGrantResource.go
package authzapi

import (
	"strings"

	"klynx/internal/services/authzsvc"
	"klynx/models/authzmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// GrantResourceHandler godoc
// @Summary      Grant permission relationships to resource
// @Description  Grant owner, editor, viewer, assigned group/role, and attributes (type, crud) to a resource entity
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        request body authzmod.GrantResourceRequest true "Grant resource permission payload"
// @Success      200 {object} gmod.SuccessMessageResponse "Resource granted successfully"
// @Failure      400 {object} gmod.ErrorMessageResponse "Invalid input or missing fields"
// @Failure      500 {object} gmod.ErrorMessageResponse "Internal error from Permify"
// @Router       /authz/resource/grant [post]
// @Security     BearerAuth
func GrantResourceHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.GrantResourceHandler")
	defer span.End()

	var req authzmod.GrantResourceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}

	if strings.TrimSpace(req.ResourceID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "resourceId is required",
			Status:  false,
		})
	}

	if err := authzsvc.GrantResource(ctx, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "✅ Resource granted successfully",
		Status:  true,
	})
}
