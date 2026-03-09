// controllers/usrapi/create.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.usrapi", "CreateUser", "usrapi", "CreateUser")
	defer end()

	var req usrmod.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return httputil.FailBadRequest(c, err.Error())
	}

	if req.Username == "" || req.Password == "" {
		return httputil.FailBadRequest(c, "username or password missing")
	}

	if err := usrsvc.CreateUser(ctx, req); err != nil {
		log.Error().Err(err).Msg("❌ Create user failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("username", req.Username).Msg("✅ CreateUser success")

	return httputil.MessageOK(c, "user created")
}
