// controllers/authapi/signin.go
package authapi

import (
	"klynx/internal/services/authsvc"
	"klynx/models/authmod"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

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
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/authapi", "authentication.Signin", "authapi", "Signin")
	defer span.End()
	var req authmod.SigninRequest
	if err := c.BodyParser(&req); err != nil {

		log.Info().
			Err(err).
			Msg("❌ [Signin] Invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", err.Error())
	}

	log.Debug().Str("username", req.Username).Msg("🔐 [Signin] Attempt login")

	resp, err := authsvc.Authenticate(ctx, req)
	if err != nil {
		log.Warn().
			Str("username", req.Username).Err(err).
			Msg("❌ [Signin] Authentication failed")
		return httputil.FailUnauthorized(c, "AUTH_FAILED", err.Error())
	}

	log.Debug().
		Str("username", req.Username).Msg("✅ [Signin] Authentication successful")
	return gmod.SendSuccess(c, resp)
}
