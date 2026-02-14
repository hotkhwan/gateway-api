// utils/traceutil/obsutil.go
package traceutil

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"klynx/internal/logger"
)

// Start: สร้าง span + คืน ctx และ logger ที่ผูก traceId/spanId ให้เรียบร้อย
// ใช้เมื่อเริ่มต้น handler/service
func Start(parent context.Context, tracerName, spanName, component, source string) (context.Context, trace.Span, zerolog.Logger) {
	ctx, span := otel.Tracer(tracerName).Start(parent, spanName)
	log := logger.FromCtx(ctx, component, source)
	return ctx, span, log
}

// Async: รันงานย่อยใน goroutine พร้อม timeout, propagate ctx, log error ให้อัตโนมัติ
// name ใช้ติด prefix ของ log ให้รู้ว่าเป็นงานอะไร
func Async(parent context.Context, timeout time.Duration, name string, fn func(context.Context) error, log zerolog.Logger) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("async", name).Interface("panic", r).Msg("panic recovered")
			}
		}()
		ctx := parent
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parent, timeout)
			defer cancel()
		}
		if err := fn(ctx); err != nil {
			log.Warn().Str("async", name).Err(err).Msg("async task failed")
		}
	}()
}

// StartLite: เริ่ม span + คืน ctx, end(), และ logger ที่ติด traceId/spanId
func StartLite(parent context.Context, tracerName, spanName, component, source string) (context.Context, func(), zerolog.Logger) {
	ctx, span := otel.Tracer(tracerName).Start(parent, spanName)
	log := logger.FromCtx(ctx, component, source)
	end := func() { span.End() }
	return ctx, end, log
}
