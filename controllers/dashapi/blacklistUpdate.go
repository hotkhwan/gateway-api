// controllers/dashapi/blacklistUpdate.go
package dashapi

import (
	"klynx/internal/services/dashsvc"
	"klynx/models/dashmod"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
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
func UpdateBlacklist(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"klynx/dashapi",
		"dashboard.UpdateBlacklist",
		"dashapi", "UpdateBlacklist",
	)
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": false, "message": "id is required"})
	}

	var body dashmod.UpdateBlacklistReq
	if err := c.BodyParser(&body); err != nil {
		log.Error().Err(err).Msg("❌ invalid body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": false, "message": "invalid request body"})
	}
	body.ID = id

	if err := dashsvc.UpdateBlacklistEvent(ctx, body); err != nil {
		log.Error().Err(err).Msg("❌ failed to update blacklist")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"status": true, "message": "blacklist updated"})
}
