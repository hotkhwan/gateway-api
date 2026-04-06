// controllers/authapi/oauthCallback.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
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
func OAuthCallback(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.authapi", "OAuthCallback.OAuthCallback", "authapi", "OAuthCallback")
	defer end()

	var req authmod.OAuthCallbackRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Info().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", err.Error())
	}

	if req.Code == "" || req.RedirectUri == "" {
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "missing code or redirectUri")
	}

	resp, err := authsvc.AuthenticateOAuthCode(ctx, req)
	if err != nil {
		log.Warn().Err(err).Msg("OAuth exchange failed")
		return httputil.FailUnauthorized(c, "AUTH_FAILED", err.Error())
	}

	return httputil.Ok(c, resp)
}
