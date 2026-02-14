// controllers/thirdapi/token.go
package thirdapi

import (
	"klynx/internal/logger"
	"klynx/internal/services/thirdsvc"
	"klynx/models/thirdmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
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
func ClientCredentialsToken(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/thirdapi")
	ctx, span := tracer.Start(ctx, "ThirdAPI.ClientCredentialsToken")
	defer span.End()

	log := logger.FromCtx(ctx, "thirdapi", "ClientCredentialsToken")

	var req thirdmod.ClientCredentialsReq
	if err := c.BodyParser(&req); err != nil {
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
