// controllers/usrapi/update.go
package usrapi

import (
	"klynx/internal/services/usrsvc"
	"klynx/models/gmod"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// UpdateUser godoc
// @Summary      Update user attributes
// @Description  Update user attributes in Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Param        body  body   object  true  "User attributes (attributes only)"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id} [patch]
func UpdateUser(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(
		c.UserContext(),
		"klynx/usrapi", "users.UpdateUser",
		"usrapi", "UpdateUser",
	)
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "MISSING_ID", Message: "missing user id", Status: false,
		})
	}

	var attrs map[string]any
	if err := c.BodyParser(&attrs); err != nil {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code: "INVALID_BODY", Message: err.Error(), Status: false,
		})
	}

	if err := usrsvc.UpdateUser(ctx, id, attrs); err != nil {
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code: "UPDATE_USER_FAILED", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "user updated", Status: true,
	})
}
