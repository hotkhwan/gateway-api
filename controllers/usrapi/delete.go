// controllers/usrapi/delete.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeleteUser godoc
// @Summary      Delete user
// @Description  Delete user from Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id} [delete]
func DeleteUser(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/usrapi", "users.DeleteUser",
		"usrapi", "DeleteUser",
	)
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "MISSING_ID", Message: "missing user id", Status: false,
		})
	}

	if err := usrsvc.DeleteUser(ctx, id); err != nil {
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code: "DELETE_USER_FAILED", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "user deleted", Status: true,
	})
}
