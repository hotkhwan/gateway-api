// controllers/kctrlapi/resolvedAlarm.go
package kctrlapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ResolvedAlarm godoc
// @Summary      Resolve alarm
// @Description  Mark alarm as resolved and append SOP resolve step (append-only). Optionally auto-close inProgress SOP steps by appending update steps.
// @Tags         KControl Alarms
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string  true  "Alarm ID"
// @Param        body  body   kctrlmod.ResolvedAlarmRequest  true  "Resolve payload"
// @Success      200   {object}  gmod.SuccessDetailResponseWithMSG
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /kcontrol/alarms/{id}/resolved [patch]
func ResolvedAlarm(c *fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c.UserContext(), "gateway.kctrlapi", "alarms.ResolvedAlarm", "kctrlapi", "ResolvedAlarm")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing id")
	}

	var req kctrlmod.ResolvedAlarmRequest
	if err := c.BodyParser(&req); err != nil {
		return httputil.FailBadRequest(c, err.Error())
	}

	res, err := kctrlsvc.ResolvedAlarm(c.UserContext(), id, req, c)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("ResolvedAlarm failed")
		return httputil.FailBadRequest(c, err.Error())
	}

	return httputil.Ok(c, res, "OK")
}
