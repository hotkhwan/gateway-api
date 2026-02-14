// utils/httputil/success.go
package httputil

import (
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
)

// Ok ส่ง 200 พร้อม code/message/และ payload เป็น data (กลางๆ ใช้ได้ทุกที่)
func Ok(c *fiber.Ctx, data any, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusOK).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeSuccess,
		Message: msg,
		Status:  true,
		Data:    data,
	})
}

// Created ส่ง 201 (เหมาะกับ create)
func Created(c *fiber.Ctx, data any, message ...string) error {
	msg := "created"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeCreated,
		Message: msg,
		Status:  true,
		Data:    data,
	})
}

// Accepted ส่ง 202 (เหมาะกับ async job)
func Accepted(c *fiber.Ctx, data any, message ...string) error {
	msg := "accepted"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusAccepted).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeAccepted,
		Message: msg,
		Status:  true,
		Data:    data,
	})
}

// MessageOK ส่ง 200 แบบไม่มี data (ใช้แทน SendMessageOK ทีหลังได้)
func MessageOK(c *fiber.Ctx, message string, code ...string) error {
	cde := gmod.CodeSuccess
	if len(code) > 0 && code[0] != "" {
		cde = code[0]
	}

	return c.Status(fiber.StatusOK).JSON(gmod.SuccessMessageResponse{
		Code:    cde,
		Message: message,
		Status:  true,
	})
}
