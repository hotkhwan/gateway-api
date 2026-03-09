// controllers/usrapi/enable.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// EnableUser godoc
// @Summary      Enable user
// @Description  Enable user in Keycloak (enabled = true)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/enable [patch]
func EnableUser(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.usrapi", "EnableUser", "usrapi", "EnableUser")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing user id")
	}

	if err := usrsvc.SetUserEnabled(ctx, id, true); err != nil {
		log.Error().Err(err).Str("userId", id).Msg("❌ EnableUser failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("userId", id).Msg("✅ EnableUser success")

	return httputil.MessageOK(c, "user enabled")
}
