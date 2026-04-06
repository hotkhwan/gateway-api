// controllers/thirdapi/token.go
package thirdapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/thirdsvc"
	"github.com/hotkhwan/gateway-api/models/thirdmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// @Summary Get access token via client_credentials
// @Tags Third-Party
// @Accept json
// @Produce json
// @Param data body thirdmod.ClientCredentialsReq true "client_id & client_secret"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /token/api/clientCredentials [post]
func ClientCredentialsToken(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.thirdapi", "ThirdAPI.ClientCredentialsToken", "thirdapi", "ClientCredentialsToken")
	defer end()

	var req thirdmod.ClientCredentialsReq
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "invalid json body")
	}
	if req.ClientID == "" || req.ClientSecret == "" {
		return httputil.FailBadRequest(c, "client_id and client_secret are required")
	}
	if req.Scope == "" {
		req.Scope = "openid"
	}
	token, err := thirdsvc.GetClientCredentialsToken(ctx, req.ClientID, req.ClientSecret, req.Scope)
	if err != nil {
		log.Error().Err(err).Msg("client_credentials token failed")
		return httputil.FailUnauthorized(c, "invalid client credentials")
	}

	return httputil.Ok(c, token)
}
