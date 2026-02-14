// controllers/authapi/oauthCallback.go
package authapi

import (
	"klynx/internal/services/authsvc"
	"klynx/models/authmod"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// @Summary OAuth callback exchange (authorization_code -> tokens)
// @Tags Auth
// @Accept json
// @Produce json
// @Param data body authmod.OAuthCallbackRequest true "OAuth callback payload"
// @Success 200 {object} authmod.SigninResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /auth/oauth/callback [post]
func OAuthCallback(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/authapi", "authentication.OAuthCallback", "authapi", "OAuthCallback")
	defer span.End()

	var req authmod.OAuthCallbackRequest
	if err := c.BodyParser(&req); err != nil {
		log.Info().Err(err).Msg("❌ [OAuthCallback] Invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", err.Error())
	}

	if req.Code == "" || req.RedirectUri == "" {
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "missing code or redirectUri")
	}

	resp, err := authsvc.AuthenticateOAuthCode(ctx, req)
	if err != nil {
		log.Warn().Err(err).Msg("❌ [OAuthCallback] Exchange failed")
		return httputil.FailUnauthorized(c, "AUTH_FAILED", err.Error())
	}

	return gmod.SendSuccess(c, resp)
}
