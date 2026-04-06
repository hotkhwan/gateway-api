// controllers/authapi/resetPassword.go
package authapi

import (
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
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
func ResetPassword(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.authapi", "ResetPassword.ResetPassword", "authapi", "ResetPassword")
	defer end()

	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "INVALID_REQUEST", "Invalid request")
	}

	accessToken, err := middleware.ExtractBearerToken(c)
	if err != nil {
		log.Warn().Err(err).Msg("missing bearer token")
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", "Missing or invalid Authorization header")
	}

	log.Info().Msg("attempting password reset")

	resp, err := authsvc.ResetPassword(ctx, "", accessToken, req.NewPassword)
	if err != nil {
		log.Warn().Err(err).Msg("password reset failed")
		return httputil.FailUnauthorized(c, "RESET_FAILED", "Password reset failed")
	}

	log.Info().Msg("password reset successful")
	return httputil.Ok(c, resp)
}
