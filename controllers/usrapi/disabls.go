// controllers/usrapi/disabls.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// DisableUser godoc
// @Summary      Disable user
// @Description  Disable user in Keycloak (enabled = false)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id}/disable [patch]
func DisableUser(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.usrapi", "DisableUser", "usrapi", "DisableUser")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing user id")
	}

	if err := usrsvc.SetUserEnabled(ctx, id, false); err != nil {
		log.Error().Err(err).Str("userId", id).Msg("❌ DisableUser failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("userId", id).Msg("✅ DisableUser success")

	return httputil.MessageOK(c, "user disabled")
}
