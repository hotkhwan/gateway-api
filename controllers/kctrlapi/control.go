// controllers/kctrlapi/control.go
package kctrlapi

import (
	"klynx/internal/services/kctrlsvc"
	"klynx/models/gmod"
	"klynx/models/kctrlmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// MessageKcontrolDevice godoc
// @Summary      Send message to Kcontrol device
// @Description  Accepts an EventMessage and publishes it as JSON to MQTT
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        body  body      kctrlmod.ControlMessage  true  "Message Payload"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /kcontrol [post]
func SendMessage(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "messages.SendMessage", "kctrlapi", "SendMessage")
	defer span.End()

	var msg kctrlmod.ControlMessage
	if err := c.BodyParser(&msg); err != nil {
		log.Warn().Err(err).Msg("❌ [SendMessage] Invalid request body")
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	// log payload ทั้งก้อน (HwID อาจเป็น string หรือ []string)
	log.Info().
		Interface("payload", msg).
		Msg("📡 [SendMessage] Sending control message to device")

	if err := kctrlsvc.SendMessage(ctx, msg); err != nil {
		log.Error().
			Err(err).
			Interface("hwId", msg.HwID). // << เปลี่ยนจาก .Str
			Msg("❌ [SendMessage] Failed to send message via MQTT")
		return httputil.FailBadRequest(c, "MQTT_SEND_FAILED", "Failed to send message via MQTT")
	}

	log.Info().
		Interface("hwId", msg.HwID). // << เปลี่ยนจาก .Str
		Msg("✅ [SendMessage] Message sent successfully")

	return gmod.SendMessageOK(c, "KCONTROL_SEND_SUCCESS", "Message sent to device successfully")
}
