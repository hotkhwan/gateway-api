// internal/kafka/producer.go
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"klynx/config"
	"klynx/internal/logger"
	"klynx/utils/traceutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrEmptyTopic = errors.New("empty topic")
	ErrEmptyKey   = errors.New("empty key")
)

// ---------------------
// Public API
// ---------------------

func PublishEventTo(ctx context.Context, topic, key string, event any, headers map[string]string) error {
	log := logger.WithMeta("kafka", "producer-PublishEventTo")

	if topic == "" {
		return ErrEmptyTopic
	}
	if key == "" {
		return ErrEmptyKey
	}

	// 1) ensure ctx has reasonable timeout (respect existing deadlines)
	ctx2, cancel := ensureTimeout(ctx, defaultPublishTimeout())
	defer cancel()

	// 2) ensure standard headers exist (merge if caller not set)
	std := standardHeadersFrom(ctx2, event, headers)
	for k, v := range std {
		if headers == nil {
			headers = make(map[string]string, 6)
		}
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}

	// 3) ✅ Inject W3C trace context ลง headers (traceparent/tracestate)
	if headers == nil {
		headers = make(map[string]string, 4)
	}
	carrier := propagation.MapCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx2, carrier)

	// 4) marshal payload
	b, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Msg("❌ marshal event failed")
		return err
	}

	// preview log
	log.Debug().
		Str("topic", topic).
		Str("key", key).
		RawJSON("payload", b).
		Msg("🧪 Kafka payload before send")

	// 5) send via centralized writer
	if err := config.SendToKafkaWithCtx(ctx2, topic, key, b, headers); err != nil {
		log.Error().Err(err).Str("topic", topic).Str("key", key).Msg("❌ Kafka write failed")
		return err
	}

	log.Debug().
		Str("topic", topic).
		Str("key", key).
		Msg("✅ Kafka write ok")

	return nil
}

// ---------------------
// Helpers (internal)
// ---------------------

// defaultPublishTimeout returns timeout from env `KAFKA_PUBLISH_TIMEOUT` or 5s.
func defaultPublishTimeout() time.Duration {
	if v := os.Getenv("KAFKA_PUBLISH_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 5 * time.Second
}

// ensureTimeout: if ctx has no deadline, wrap with timeout d. If already has deadline, keep it.
func ensureTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, has := ctx.Deadline(); has {
		// keep original; return a no-op cancel
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// standardHeadersFrom builds standard headers if missing.
// You can adapt this to read schema/source from env.
func standardHeadersFrom(ctx context.Context, event any, _ map[string]string) map[string]string {
	// extract trace id from ctx (prefer OTel, then util fallback)
	traceID := traceIDFromCtx(ctx)

	// detect minimal fields from known event shapes (best-effort)
	type hasMeta struct {
		Event string `json:"event"`
		ID    string `json:"id"`
		Rev   int64  `json:"rev"`
	}
	ev := hasMeta{}
	_ = json.Unmarshal(mustJSON(event), &ev) // best-effort; ignore error

	// source/schema may be global for the service; set via env
	source := os.Getenv("EVENT_SOURCE")
	if source == "" {
		source = "klynx"
	}
	schema := os.Getenv("EVENT_SCHEMA") // e.g. "kwatch/watchlist/1"
	if schema == "" {
		schema = "klynx/default/1"
	}
	idem := ""
	if ev.Event != "" && ev.ID != "" {
		idem = ev.Event + ":" + ev.ID + ":" + itoa(ev.Rev)
	}

	out := map[string]string{
		"event":           ev.Event,
		"rev":             itoa(ev.Rev),
		"source":          source,
		"schema":          schema,
		"traceId":         traceID, // ใช้รูปแบบเดียวกับ logger.FromCtx
		"idempotency_key": idem,
	}

	// drop empty fields so we don't overwrite caller's non-empty ones later
	for k, v := range out {
		if v == "" {
			delete(out, k)
		}
	}
	return out
}

func traceIDFromCtx(ctx context.Context) string {
	// 1) OTel span context (ดีที่สุด)
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	// 2) our util fallback (ถ้าใครบันทึกไว้เอง)
	if tid := traceutil.TraceIDFromCtx(ctx); tid != "" {
		return tid
	}
	// 3) legacy key (รองรับของเก่า)
	type ctxKey string
	if v := ctx.Value(ctxKey("traceid")); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func itoa(x int64) string {
	return strconv.FormatInt(x, 10)
}
