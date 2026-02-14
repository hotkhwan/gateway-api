// internal/middleware/method.go
package middleware

import (
	"klynx/internal/logger"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func AllowOnly(method string) fiber.Handler { // Remide remove this function
	return func(c *fiber.Ctx) error {
		if c.Method() != method {
			log := logger.FromCtx(c.UserContext(), "middleware", "AllowOnly")
			log.Warn().
				Str("method", c.Method()).
				Str("allowed", method).
				Str("path", c.OriginalURL()).
				Msg("method_not_allowed")

			return c.Status(fiber.StatusMethodNotAllowed).JSON(fiber.Map{
				"message": "Method Not Allowed",
				"method":  c.Method(),
				"path":    c.OriginalURL(),
				"status":  false,
			})
		}
		return c.Next()
	}
}

func AllowMethods(allowed ...string) fiber.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, m := range allowed {
		set[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		// CORS preflight ผ่านไปก่อน (ถ้ามี CORS middleware อยู่หน้าสุด ให้ลบส่วนนี้ได้)
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		if _, ok := set[c.Method()]; !ok {
			return c.Status(fiber.StatusMethodNotAllowed).JSON(fiber.Map{
				"message": "Method Not Allowed",
				"method":  c.Method(),
				// "allowed": allowed,
				"path":   c.OriginalURL(),
				"status": false,
			})
		}
		return c.Next()
	}
}
