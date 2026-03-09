// controllers/usrapi/delete.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeleteUser godoc
// @Summary      Delete user
// @Description  Delete user from Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id} [delete]
func DeleteUser(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.usrapi", "DeleteUser", "usrapi", "DeleteUser")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing user id")
	}

	if err := usrsvc.DeleteUser(ctx, id); err != nil {
		log.Error().Err(err).Str("userId", id).Msg("❌ DeleteUser failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("userId", id).Msg("✅ DeleteUser success")

	return httputil.MessageOK(c, "user deleted")
}
