// controllers/authzapi/permisstionCheck.go
package authzapi

import (
	"fmt"

	"klynx/internal/services/authzsvc"
	"klynx/models/authzmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// PermissionCheckHandler godoc
// @Summary      Check User Permission
// @Description  Check if the user has permission to perform an action on a resource
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        body body authzmod.PermissionCheckRequest true "Permission check request"
// @Success      200 {object} gmod.SuccessMessageResponse "Permission check result"
// @Failure      400 {object} gmod.SuccessMessageResponse "Invalid request body"
// @Failure      500 {object} gmod.SuccessMessageResponse "Internal server error"
// @Router       /authz/permission/check [post]
// @Security     BearerAuth
func PermissionSubjectCheckHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.PermissionCheckHandler")
	defer span.End()

	var req authzmod.PermissionSubjectCheckRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.SuccessMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid request body",
			Status:  false,
		})
	}

	allowed, err := authzsvc.PermissionSubjectCheck(ctx, req)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageResponse{
			Code:    "ERROR",
			Message: fmt.Sprintf("permission check failed: %v", err),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessDataResponse{
		Code:   "SUCCESS",
		Status: true,
		Data:   allowed,
	})
}
