// controllers/usrapi/changePassword.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// ChangePassword godoc
// @Summary      Change user password
// @Description  Change user password in Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Param        body  body   usrmod.ChangePasswordRequest  true  "New password"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/password [patch]
func ChangePassword(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.usrapi", "ChangePassword", "usrapi", "ChangePassword")
	defer end()

	id := c.Params("id")

	var req struct {
		Password  string `json:"password"`
		Temporary bool   `json:"temporary"`
	}

	if err := c.Bind().Body(&req); err != nil || req.Password == "" {
		return httputil.FailBadRequest(c, "password required")
	}

	if err := usrsvc.ChangePassword(ctx, id, req.Password, req.Temporary); err != nil {
		log.Error().Err(err).Str("userId", id).Msg("❌ ChangePassword failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("userId", id).Msg("✅ ChangePassword success")

	return httputil.MessageOK(c, "password changed")
}
