// controllers/authapi/signout.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.authapi", "Signout.Signout", "authapi", "Signout")
	defer end()

	accessToken, err := middleware.ExtractBearerToken(c)
	if err != nil {
		log.Warn().Err(err).Msg("missing bearer token")
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	log.Info().Msg("attempting signout")

	resp, err := authsvc.Signout(ctx, accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("signout failed")
		return httputil.FailUnauthorized(c, "SIGNOUT_FAILED", err.Error())
	}

	if msg, ok := resp["message"].(string); ok {
		log.Info().Msg("signout successful")
		return httputil.MessageOK(c, msg, "SIGNOUT_SUCCESS")
	}

	log.Error().Msg("unexpected response format")
	return httputil.FailInternal(c, "UNEXPECTED_RESPONSE", "Invalid message format")
}
