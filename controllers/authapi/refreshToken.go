// controllers/authapi/refreshToken.go
package authapi

import (
	"klynx/internal/logger"
	"klynx/internal/services/authsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Summary Refresh Token
// @Tags Auth
// @tag.order 1
// @Accept json
// @Produce json
// @Param data body map[string]string true "Refresh Token"
// @Success 200 {object} authmod.SigninResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /auth/refreshToken [post]
// @Security BearerAuth
func RefreshToken(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authapi")
	ctx, span := tracer.Start(ctx, "Auth.RefreshToken")
	defer span.End()

	log := logger.FromCtx(ctx, "authapi", "RefreshToken")

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "Invalid request")
	}
	if req.RefreshToken == "" {
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "refresh_token is required")
	}

	log.Debug().Msg("attempting token refresh")

	// ✅ ส่ง ctx เข้า service เพื่อ chain trace ต่อ
	resp, err := authsvc.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		log.Warn().Err(err).Msg("token refresh failed")
		return httputil.FailUnauthorized(c, "TOKEN_INVALID", "Refresh token is invalid or expired")
	}

	log.Debug().Msg("token refresh successful")
	return gmod.SendSuccess(c, resp)
}
