// controllers/usrapi/disabls.go
package usrapi

import (
	"klynx/internal/services/usrsvc"
	"klynx/models/gmod"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DisableUser godoc
// @Summary      Disable user
// @Description  Disable user in Keycloak (enabled = false)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/disable [patch]
func DisableUser(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(
		c.UserContext(),
		"klynx/usrapi", "users.DisableUser",
		"usrapi", "DisableUser",
	)
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "MISSING_ID", Message: "missing user id", Status: false,
		})
	}

	if err := usrsvc.SetUserEnabled(ctx, id, false); err != nil {
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code: "DISABLE_USER_FAILED", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "user disabled", Status: true,
	})
}
