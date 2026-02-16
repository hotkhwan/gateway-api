// controllers/usrapi/enable.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// EnableUser godoc
// @Summary      Enable user
// @Description  Enable user in Keycloak (enabled = true)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/enable [patch]
func EnableUser(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/usrapi", "users.EnableUser",
		"usrapi", "EnableUser",
	)
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "MISSING_ID", Message: "missing user id", Status: false,
		})
	}

	if err := usrsvc.SetUserEnabled(ctx, id, true); err != nil {
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code: "ENABLE_USER_FAILED", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "user enabled", Status: true,
	})
}
