// controllers/authapi/signin.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// @Summary Signin
// @Tags Auth
// @Accept json
// @Produce json
// @Param data body authmod.SigninRequest true "User credentials"
// @Success 200 {object} authmod.SigninResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /auth/signin [post]
func Signin(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.authapi", "Signin.Signin", "authapi", "Signin")
	defer end()

	var req authmod.SigninRequest
	if err := c.BodyParser(&req); err != nil {
		log.Info().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", err.Error())
	}

	log.Debug().Str("username", req.Username).Msg("attempting login")

	resp, err := authsvc.Authenticate(ctx, req)
	if err != nil {
		log.Warn().Str("username", req.Username).Err(err).Msg("authentication failed")
		return httputil.FailUnauthorized(c, "AUTH_FAILED", err.Error())
	}

	log.Debug().Str("username", req.Username).Msg("authentication successful")
	return httputil.Ok(c, resp)
}
