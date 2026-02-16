// controllers/authapi/signout.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// @Summary Signout
// @Tags Auth
// @tag.order 1
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Success 200 {object} authmod.SigninResponseWrapper
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /auth/signout [post]
// @Security BearerAuth
func Signout(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/authapi", "authentication.Signin", "authapi", "Signin")
	defer span.End()

	accessToken, err := middleware.ExtractBearerToken(c)
	if err != nil {
		log.Warn().
			Err(err).Msg("❌ [Signout] Missing bearer token")
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	log.Info().Msg("🚪 [Signout] Attempting signout")

	resp, err := authsvc.Signout(ctx, accessToken)
	if err != nil {
		log.Warn().
			Err(err).Msg("❌ [Signout] Failed to sign out")
		return httputil.FailUnauthorized(c, "SIGNOUT_FAILED", err.Error())
	}

	if msg, ok := resp["message"].(string); ok {
		log.Info().
			Msg("✅ [Signout] Signout successful")
		return gmod.SendMessageOK(c, "SIGNOUT_SUCCESS", msg)
	}

	log.Error().
		Msg("❌ [Signout] Unexpected response format")
	return httputil.FailInternal(c, "UNEXPECTED_RESPONSE", "Invalid message format")
}
