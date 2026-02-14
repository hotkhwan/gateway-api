package kschsvc

import (
	"bytes"
	"context"
	"io"
)

type ctxKey string

const (
	videoColl     = "ksearch_videos"
	videoBucket   = "ksearch"
	maxVideoBytes = int64(10 * 1024 * 1024 * 1024) // 10 GiB
)

const traceIDKey ctxKey = "trace_id"

func traceIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(traceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// (ไม่ต้องใช้ แต่เผื่อ mock reader ใน unit test)
var _ = bytes.MinRead
var _ io.Reader
