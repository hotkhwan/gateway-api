// controllers/authapi/refreshToken.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
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
func RefreshToken(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.authapi", "RefreshToken.RefreshToken", "authapi", "RefreshToken")
	defer end()

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind().Body(&req); err != nil {
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
	return httputil.Ok(c, resp)
}
