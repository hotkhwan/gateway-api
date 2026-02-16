// controllers/usrapi/create.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// CreateUser godoc
// @Summary      Create new user
// @Description  Create new user in Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  usrmod.CreateUserRequest  true  "Create user payload"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users [post]
func CreateUser(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/usrapi", "users.CreateUser",
		"usrapi", "CreateUser",
	)
	defer span.End()

	var req usrmod.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code:    "INVALID_BODY",
			Message: err.Error(),
			Status:  false,
		})
	}

	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(gmod.ErrorResponse{
			Code:    "MISSING_REQUIRED_FIELD",
			Message: "username or password missing",
			Status:  false,
		})
	}

	if err := usrsvc.CreateUser(ctx, req); err != nil {
		log.Error().Err(err).Msg("❌ Create user failed")
		return c.Status(500).JSON(gmod.ErrorResponse{
			Code:    "CREATE_USER_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code:    "SUCCESS",
		Message: "user created",
		Status:  true,
	})
}
