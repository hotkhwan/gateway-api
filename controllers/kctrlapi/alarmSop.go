// controllers/kctrlapi/alarmSop.go
package kctrlapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

func AppendAlarmSop(c *fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c.UserContext(), "gateway.kctrlapi", "alarms.AppendAlarmSop", "kctrlapi", "AppendAlarmSop")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "missing id")
	}

	var req kctrlmod.SopStepRequest
	if err := c.BodyParser(&req); err != nil {
		return httputil.FailBadRequest(c, err.Error())
	}

	if err := kctrlsvc.AppendAlarmSop(c.UserContext(), id, req, c); err != nil {
		log.Error().Err(err).Str("id", id).Msg("AppendAlarmSop failed")
		return httputil.FailBadRequest(c, err.Error())
	}

	return httputil.MessageOK(c, "OK")
}
