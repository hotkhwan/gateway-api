// utils/traceutil/scope.go
package traceutil

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

// Scope: เก็บ ctx, span, logger สำหรับหนึ่งงาน (service/handler/consumer)
type Scope struct {
	Ctx      context.Context
	Span     trace.Span
	Log      zerolog.Logger
	tracer   string
	spanName string
	comp     string
	source   string
}

// StartScope: ระบุได้หมด — tracerName, spanName, component, source
// ตัวอย่าง:
//
//	s := obs.StartScope(parent, "github.com/hotkhwan/gateway-api/kwatsvc", "kwatsvc.WatchlistCreate", "kwatsvc", "WatchlistCreate")
//	s := obs.StartScope(parent, "github.com/hotkhwan/gateway-api/kxx",     "kxx.Xx",                 "kxx",     "Xx")
func StartScope(parent context.Context, tracerName, spanName, component, source string) *Scope {
	ctx, span := otel.Tracer(tracerName).Start(parent, spanName)
	log := logger.FromCtx(ctx, component, source)
	return &Scope{
		Ctx: ctx, Span: span, Log: log,
		tracer: tracerName, spanName: spanName, comp: component, source: source,
	}
}

// End: ปิด span
func (s *Scope) End() { s.Span.End() }

// Timer: วัดเวลางานย่อยแบบง่าย
//
//	stop := s.Timer("s3_upload"); ... ; stop()
func (s *Scope) Timer(label string) func() {
	start := time.Now()
	return func() { s.Log.Info().Dur("took", time.Since(start)).Msg(label) }
}

// TraceID: ดึง traceId (เผื่ออยากใส่เพิ่มใน log)
func (s *Scope) TraceID() string {
	if sc := trace.SpanContextFromContext(s.Ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// With: คืน scope ใหม่ที่ Log ถูก bind fields เพิ่ม (ไม่บังคับใช้ แต่สะดวก)
func (s *Scope) With(kv map[string]string) *Scope {
	b := s.Log.With()
	for k, v := range kv {
		b = b.Str(k, v)
	}
	ns := *s
	ns.Log = b.Logger()
	return &ns
}
