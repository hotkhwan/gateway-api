// utils/httputil/error.go
package httputil

import (
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
)

type ErrorDetails map[string]any

// Fail sends a generic error response with any HTTP status code.
// Use the shortcut functions below when possible.
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

// FailReason sends an error response with a reason string embedded in details.
func FailReason(c *fiber.Ctx, httpStatus int, code, message, reason string, details ...ErrorDetails) error {
	d := ErrorDetails{
		"reason": reason,
	}
	if len(details) > 0 {
		for k, v := range details[0] {
			d[k] = v
		}
	}
	return Fail(c, httpStatus, code, message, d)
}

// ---- Shortcuts ----

// FailBadRequest sends 400 Bad Request.
func FailBadRequest(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusBadRequest, gmod.CodeBadRequest, message, details...)
}

// FailUnauthorized sends 401 Unauthorized.
func FailUnauthorized(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusUnauthorized, gmod.CodeUnauthorized, message, details...)
}

// FailForbidden sends 403 Forbidden.
func FailForbidden(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusForbidden, gmod.CodeForbidden, message, details...)
}

// FailNotFound sends 404 Not Found.
func FailNotFound(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusNotFound, gmod.CodeNotFound, message, details...)
}

// FailConflict sends 409 Conflict.
func FailConflict(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusConflict, gmod.CodeConflict, message, details...)
}

// FailUnprocessable sends 422 Unprocessable Entity.
func FailUnprocessable(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, details...)
}

// FailLocked sends 423 Locked.
func FailLocked(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusLocked, "LOCKED", message, details...)
}

// FailTooMany sends 429 Too Many Requests.
func FailTooMany(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusTooManyRequests, gmod.CodeTooMany, message, details...)
}

// FailInternal sends 500 Internal Server Error.
func FailInternal(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusInternalServerError, gmod.CodeInternalError, message, details...)
}

// FailBadGateway sends 502 Bad Gateway.
func FailBadGateway(c *fiber.Ctx, message string, details ...any) error {
	return Fail(c, fiber.StatusBadGateway, "BAD_GATEWAY", message, details...)
}

// ---- Reason shortcuts ----

// FailBadRequestReason sends 400 Bad Request with a reason in details.
func FailBadRequestReason(c *fiber.Ctx, message, reason string, details ...ErrorDetails) error {
	return FailReason(c, fiber.StatusBadRequest, gmod.CodeBadRequest, message, reason, details...)
}

// FailInternalReason sends 500 Internal Server Error with a reason in details.
func FailInternalReason(c *fiber.Ctx, message, reason string, details ...ErrorDetails) error {
	return FailReason(c, fiber.StatusInternalServerError, gmod.CodeInternalError, message, reason, details...)
}
