// controllers/dashapi/blacklistUpdate.go
package dashapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/dashsvc"
	"github.com/hotkhwan/gateway-api/models/dashmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// UpdateBlacklist godoc
// @Summary Update blacklist event (acknowledged / isDeleted / sop)
// @Description Patch blacklist event by id
// @Tags Dashboard
// @Accept json
// @Produce json
// @Param id path string true "Blacklist Event ID"
// @Param body body dashmod.UpdateBlacklistReq true "Update payload"
// @Success 200 {object} map[string]any
// @Router /dashboard/blacklist/{id} [patch]
// @Security BearerAuth
func UpdateBlacklist(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.dashapi", "Dashboard.UpdateBlacklist", "dashapi", "UpdateBlacklist")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "id is required")
	}

	var body dashmod.UpdateBlacklistReq
	if err := c.Bind().Body(&body); err != nil {
		log.Error().Err(err).Msg("❌ invalid body")
		return httputil.FailBadRequest(c, "invalid request body")
	}
	body.ID = id

	if err := dashsvc.UpdateBlacklistEvent(ctx, body); err != nil {
		log.Error().Err(err).Msg("❌ failed to update blacklist")
		return httputil.FailBadRequest(c, err.Error())
	}

	return httputil.MessageOK(c, "blacklist updated")
}
