// internal/logger/middleware.go
package logger

import (
	"time"

	"github.com/gofiber/fiber/v3"
)

func FiberLogger() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		lat := time.Since(start)

		status := c.Response().StatusCode()
		log := FromCtx(c, "http", "request")

		ev := log.Info()
		// Downgrade successful GET/HEAD reads to Debug to reduce polling noise
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			if status >= 200 && status < 300 {
				ev = log.Debug()
			}
		}

		ev.
			Str("method", c.Method()).
			Str("path", c.OriginalURL()).
			Int("status", status).
			Str("ip", c.IP()).
			Dur("latency", lat).
			Str("userAgent", string(c.Request().Header.UserAgent())).
			Msg("http_request")

		return err
	}
}
