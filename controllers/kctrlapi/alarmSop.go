// controllers/kctrlapi/alarmSop.go
package kctrlapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"

	"github.com/gofiber/fiber/v2"
)

func AppendAlarmSop(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "INVALID_ID", Message: "missing id", Status: false,
		})
	}

	var req kctrlmod.SopStepRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "BAD_REQUEST", Message: err.Error(), Status: false,
		})
	}

	if err := kctrlsvc.AppendAlarmSop(c.UserContext(), id, req, c); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code: "ERROR", Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessResponse{
		Code: "SUCCESS", Message: "OK", Status: true,
	})
}
