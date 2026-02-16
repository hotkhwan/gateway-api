// utils/httputil/error.go
package httputil

import (
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
)

type ErrorDetails map[string]any

func Fail(c *fiber.Ctx, httpStatus int, code, message string, details ...any) error {
	var d any
	if len(details) > 0 {
		d = details[0]
	}

	return c.Status(httpStatus).JSON(gmod.ApiErrorResponse{
		Code:    code,
		Message: message,
		Details: d,
		Status:  false,
	})
}

// FailReason ทำให้ pattern เดียวกันทั้งระบบ: code กลาง + reason เฉพาะใน details
func FailReason(c *fiber.Ctx, httpStatus int, code, message, reason string, details ...ErrorDetails) error {
	d := ErrorDetails{
		"reason": reason,
	}
	if len(details) > 0 {
		// merge details[0] -> d
		for k, v := range details[0] {
			d[k] = v
		}
	}
	return Fail(c, httpStatus, code, message, d)
}

// ---- Shortcuts (normalized codes) ----

func FailBadRequest(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusBadRequest, gmod.CodeBadRequest, message, details...)
}

func FailUnauthorized(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusUnauthorized, gmod.CodeUnauthorized, message, details...)
}

func FailForbidden(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusForbidden, gmod.CodeForbidden, message, details...)
}

func FailNotFound(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusNotFound, gmod.CodeNotFound, message, details...)
}

func FailConflict(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusConflict, gmod.CodeConflict, message, details...)
}

func FailTooMany(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusTooManyRequests, gmod.CodeTooMany, message, details...)
}

func FailInternal(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusInternalServerError, gmod.CodeInternalError, message, details...)
}

// ---- Reason shortcuts (recommended) ----

func FailBadRequestReason(c *fiber.Ctx, message, reason string, details ...ErrorDetails) error {
	return FailReason(c, fiber.StatusBadRequest, gmod.CodeBadRequest, message, reason, details...)
}

func FailInternalReason(c *fiber.Ctx, message, reason string, details ...ErrorDetails) error {
	return FailReason(c, fiber.StatusInternalServerError, gmod.CodeInternalError, message, reason, details...)
}
