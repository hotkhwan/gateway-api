// utils/httputil/success.go
package httputil

import (
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
)

// Ok sends 200 OK with data payload.
func Ok(c *fiber.Ctx, data any, message ...string) error {
	msg := "ok"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusOK).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeSuccess,
		Message: msg,
		Status:  true,
		Details: data,
	})
}

// Created sends 201 Created with data payload.
func Created(c *fiber.Ctx, data any, message ...string) error {
	msg := "created"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeCreated,
		Message: msg,
		Status:  true,
		Details: data,
	})
}

// Accepted sends 202 Accepted with data payload (for async jobs).
func Accepted(c *fiber.Ctx, data any, message ...string) error {
	msg := "accepted"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	return c.Status(fiber.StatusAccepted).JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeAccepted,
		Message: msg,
		Status:  true,
		Details: data,
	})
}

// NoContent sends 204 No Content (no body).
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// MessageOK sends 200 OK with message only (no data payload).
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
