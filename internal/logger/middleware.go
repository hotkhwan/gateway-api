// internal/logger/middleware.go
package logger

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

func FiberLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		lat := time.Since(start)

		log := FromCtx(c.UserContext(), "http", "request")
		log.Info().
			Str("method", c.Method()).
			Str("path", c.OriginalURL()).
			Int("status", c.Response().StatusCode()).
			Str("ip", c.IP()).
			Dur("latency", lat).
			Str("userAgent", string(c.Request().Header.UserAgent())).
			Msg("http_request")

		return err
	}
}
