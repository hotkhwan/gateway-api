// controllers/authapi/resetPassword.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Summary Reset Password
// @Tags Auth
// @tag.order 1
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param data body map[string]string true "New Password"
// @Success 200 {object} authmod.SigninResponseWrapper
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Router /auth/resetPassword [post]
// @Security BearerAuth
func ResetPassword(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authapi")
	ctx, span := tracer.Start(ctx, "Auth.ResetPassword")
	defer span.End()

	log := logger.FromCtx(ctx, "authapi", "ResetPassword")

	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		log.Warn().
			Err(err).Msg("❌ [ResetPassword] Invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "Invalid request")
	}

	accessToken, err := middleware.ExtractBearerToken(c)
	if err != nil {
		log.Warn().
			Err(err).Msg("❌ [ResetPassword] Missing bearer token")
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", "Missing or invalid Authorization header")
	}

	log.Info().
		Msg("🔄 [ResetPassword] Attempting password reset")

	resp, err := authsvc.ResetPassword(ctx, "", accessToken, req.NewPassword)
	if err != nil {
		log.Warn().
			Err(err).Msg("❌ [ResetPassword] Reset failed")
		return httputil.FailUnauthorized(c, "RESET_FAILED", "Password reset failed")
	}

	log.Info().
		Msg("✅ [ResetPassword] Password reset successful")
	return gmod.SendSuccess(c, resp)
}
