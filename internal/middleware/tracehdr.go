// internal/middleware/tracehdr.go
package middleware

import (
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

func TraceHeader() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if sc := trace.SpanContextFromContext(c.UserContext()); sc.IsValid() {
			traceId := sc.TraceID().String()
			spanId := sc.SpanID().String()

			c.Set("X-Trace-Id", traceId)
			c.Set("X-Span-Id", spanId)
			c.Locals("traceId", traceId)
			c.Locals("spanId", spanId)
		}
		return c.Next()
	}
}
