// controllers/usrapi/update.go
package usrapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// UpdateUser godoc
// @Summary      Update user attributes
// @Description  Update user attributes in Keycloak
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "User ID"
// @Param        body  body   object  true  "User attributes (attributes only)"
// @Success      200   {object}  gmod.SuccessResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /users/{id} [patch]
func UpdateUser(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.usrapi", "UpdateUser", "usrapi", "UpdateUser")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing user id")
	}

	var attrs map[string]any
	if err := c.BodyParser(&attrs); err != nil {
		return httputil.FailBadRequest(c, err.Error())
	}

	if err := usrsvc.UpdateUser(ctx, id, attrs); err != nil {
		log.Error().Err(err).Str("userId", id).Msg("❌ UpdateUser failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().Str("userId", id).Msg("✅ UpdateUser success")

	return httputil.MessageOK(c, "user updated")
}
