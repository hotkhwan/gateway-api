// controllers/kctrlapi/resolvedAlarm.go
package kctrlapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"

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
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "INVALID_ID", Message: "missing id", Status: false,
		})
	}

	var req kctrlmod.ResolvedAlarmRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "BAD_REQUEST", Message: err.Error(), Status: false,
		})
	}

	res, err := kctrlsvc.ResolvedAlarm(c.UserContext(), id, req, c)
	if err != nil {
		// คุณอาจ map error เป็น code เฉพาะเพิ่มเองได้
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "ERROR", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessDetailResponseWithMSG{
		Code: "SUCCESS", Message: "OK", Status: true, Detail: res,
	})
}
