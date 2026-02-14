// controllers/usrapi/changePassword.go
package usrapi

import (
	"klynx/internal/services/usrsvc"
	"klynx/models/gmod"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ChangePassword godoc
// @Summary      Change user password
// @Description  Change user password in Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Param        body  body   usrmod.ChangePasswordRequest  true  "New password"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/password [patch]
func ChangePassword(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(
		c.UserContext(),
		"klynx/usrapi", "users.ChangePassword",
		"usrapi", "ChangePassword",
	)
	defer span.End()

	id := c.Params("id")

	var req struct {
		Password  string `json:"password"`
		Temporary bool   `json:"temporary"`
	}

	if err := c.BodyParser(&req); err != nil || req.Password == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "INVALID_BODY", Message: "password required", Status: false,
		})
	}

	if err := usrsvc.ChangePassword(ctx, id, req.Password, req.Temporary); err != nil {
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code: "CHANGE_PASSWORD_FAILED", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "password changed", Status: true,
	})
}
