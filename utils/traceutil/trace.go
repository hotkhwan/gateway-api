// utils/traceutil/trace.go
package traceutil

import (
	"context"
)

type ctxKey string

const traceKey ctxKey = "trace.id"

func TraceIDKey() any { return traceKey }

func WithTraceID(ctx context.Context, tid string) context.Context {
	if tid == "" {
		return ctx
	}
	return context.WithValue(ctx, traceKey, tid)
}

func TraceIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(traceKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
