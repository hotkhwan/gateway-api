// controllers/kctrlapi/delete.go
package kctrlapi

import (
	"klynx/internal/services/kctrlsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeleteAlarm godoc
// @Summary Delete an alarm event (hard delete)
// @Description Delete from events store (kcontrol_events) by _id
// @Tags Kcontrol
// @Produce json
// @Param id path string true "Alarm Event ID (_id)"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /kcontrol/alarms/{id} [delete]
// @Security BearerAuth
func DeleteAlarm(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "alarms.DeleteAlarm", "kctrlapi", "DeleteAlarm")
	defer span.End()

	id := c.Params("id")
	if id == "" {
		log.Warn().Msg("❌ [DeleteAlarm] Missing alarm ID in URL path")
		return httputil.FailBadRequest(c, "MISSING_ID", "Missing alarm ID")
	}

	log.Info().Str("alarmID", id).Msg("🗑️ [DeleteAlarm] Attempting delete (events store)")

	if err := kctrlsvc.DeleteAlarm(ctx, id); err != nil {
		log.Error().Err(err).Str("alarmID", id).Msg("❌ [DeleteAlarm] Failed to delete alarm")
		return httputil.FailInternal(c, "DELETE_FAILED", err.Error())
	}

	log.Info().Str("alarmID", id).Msg("✅ [DeleteAlarm] Alarm deleted successfully (events store)")
	return gmod.SendMessageOK(c, "ALARM_DELETE_SUCCESS", "Alarm deleted")
}

// DeleteAlarms godoc
// @Summary Delete multiple alarm events (hard delete)
// @Description Delete many from events store (kcontrol_events) by _id list
// @Tags Kcontrol
// @Accept json
// @Produce json
// @Param ids body []string true "List of Alarm Event IDs (_id) to delete"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Security BearerAuth
// @Router /kcontrol/alarms/bulk-delete [post]
func DeleteAlarmBulk(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "alarms.DeleteAlarmBulk", "kctrlapi", "DeleteAlarmBulk")
	defer span.End()

	var ids []string
	if err := c.BodyParser(&ids); err != nil || len(ids) == 0 {
		log.Warn().Msg("❌ [DeleteAlarms] Invalid or missing alarm ID list")
		return httputil.FailBadRequest(c, "INVALID_IDS", "Invalid or missing alarm ID list")
	}

	log.Info().Int("count", len(ids)).Msg("🗑️ [DeleteAlarms] Attempting bulk delete (events store)")

	if err := kctrlsvc.DeleteAlarmBulk(ctx, ids); err != nil {
		log.Error().Err(err).Msg("❌ [DeleteAlarms] Bulk delete failed")
		return httputil.FailInternal(c, "DELETE_FAILED", err.Error())
	}

	log.Info().Int("count", len(ids)).Msg("✅ [DeleteAlarms] Alarms deleted successfully (events store)")
	return gmod.SendMessageOK(c, "ALARM_BULK_DELETE_SUCCESS", "Alarms deleted")
}
