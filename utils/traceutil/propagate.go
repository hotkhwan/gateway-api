// utils/traceutil/propagate.go
package traceutil

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectHeaders: เพิ่ม traceparent/tracestate ลง header map
func InjectHeaders(ctx context.Context, dst map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, 4)
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		dst[k] = v
	}
	return dst
}

// ExtractHeaders: ดึง traceparent/tracestate จาก header map
func ExtractHeaders(parent context.Context, src map[string]string) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	carrier := propagation.MapCarrier{}
	for k, v := range src {
		carrier[k] = v
	}
	return otel.GetTextMapPropagator().Extract(parent, carrier)
}

// DetachWithParent: ยก SpanContext จาก ctx ไปใส่บน context.Background()
// ใช้ในกรณี fire-and-forget เพื่อไม่ให้โดน cancel จาก request ctx
func DetachWithParent(ctx context.Context) context.Context {
	sc := trace.SpanContextFromContext(ctx)
	bg := context.Background()
	if sc.IsValid() {
		return trace.ContextWithSpanContext(bg, sc)
	}
	return bg
}
