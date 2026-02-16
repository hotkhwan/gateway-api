// controllers/webhooks/analytic/atapi/pusher.go
package atapi

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/aimodel"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// -------------------- ATA (JSON ตรง) --------------------

func HandleATA(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/atapi")
	ctx, span := tracer.Start(ctx, "webhook.HandleATA")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "atapi", "HandleATA")

	rawBody := c.Body()

	// pretty log (จำกัดเฉพาะ dev/uat ตาม log level ของคุณ)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, rawBody, "", "  "); err == nil {
		log.Debug().Msgf("📝 ATA JSON:\n%s", pretty.String())
	} else {
		log.Warn().Err(err).Msg("Invalid JSON body (cannot indent), continue raw")
	}

	// parse เป็น dynamic map
	var msg aimodel.ATAMessage
	if err := json.Unmarshal(rawBody, &msg); err != nil {
		log.Error().Err(err).Msg("❌ Failed to parse ATAMessage")
		return httputil.FailBadRequest(c, "INVALID_FORMAT", "Invalid ATA message format")
	}

	// 🔴 ใช้ topic เฉพาะ ATA เท่านั้น (ไม่ fallback ไป KAFKA_TOPIC อีก)
	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_ATA"),
		"ata.events", // default สำหรับ ATA
	)

	// key จาก payload ATA
	key := deriveKafkaKeyFromATA(rawBody)
	env := aimodel.PusherEvent{
		Event:   "ata.forwarded",
		Channel: "", // ถ้ามีอะไรก็ใส่
		Data:    json.RawMessage(rawBody),
	}
	headers := map[string]string{
		"event":  "analytic.ata.received",
		"schema": "kai/ata/1",
	}

	log.Debug().
		Str("resolved_topic", topic).
		Msg("📤 ATA HandleATA publishing to Kafka")

	if err := kafka.PublishEventTo(ctx, topic, key, env, headers); err != nil {
		log.Error().Err(err).Str("key", key).Str("topic", topic).Msg("❌ Kafka publish failed")
		return httputil.FailInternal(c, "internal server error")
	}

	log.Debug().Str("key", key).Str("topic", topic).Msg("✅ ATA message published to Kafka")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  true,
		"code":    "SUCCESS",
		"message": "ata message queued",
		"detail": fiber.Map{
			"topic": topic,
			"key":   key,
		},
	})
}

// -------------------- Pusher-style Envelope --------------------

func HandlePusherEvent(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/atapi")
	ctx, span := tracer.Start(ctx, "webhook.HandlePusherEvent")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "atapi", "HandlePusherEvent")

	// ==== Request meta (help debug) ====
	ct := strings.ToLower(c.Get("Content-Type"))
	cl := c.Get("Content-Length")
	ua := c.Get("User-Agent")
	ip := c.IP()
	log.Debug().
		Str("ip", ip).Str("ua", ua).
		Str("contentType", ct).Str("contentLength", cl).
		Msg("📩 /atahook/pusher-event")

	// body sample (configurable)
	sampleMax := 1024
	if v := os.Getenv("LOG_WEBHOOK_BODY_SAMPLE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sampleMax = n
		}
	}
	raw := c.Body()
	if len(raw) == 0 {
		raw = c.Request().Body()
	}
	if len(raw) > 0 {
		s := raw
		if len(s) > sampleMax {
			s = s[:sampleMax]
		}
		log.Debug().Msgf("🧪 body sample(%d): %s", len(s), string(s))
	} else {
		log.Debug().Msg("🧪 body empty (might be chunked/multipart)")
	}

	var env aimodel.PusherEvent
	var got bool

	// ==== Parse by Content-Type ====
	switch {
	case strings.HasPrefix(ct, "application/json"):
		// 1) ลอง parse เป็น PusherEvent ตามปกติ
		if err := json.Unmarshal(raw, &env); err == nil {
			got = true
		}
		// 2) ถ้าไม่เจอ event/data → ถือว่าเป็น ATA JSON ทั้งก้อน
		if (env.Event == "" && len(env.Data) == 0) || !got {
			env.Event = firstNonEmpty(c.Get("X-Event"), "ata.forwarded")
			env.Channel = c.Get("X-Channel")
			env.Data = json.RawMessage(raw) // ห่อทั้งก้อนเข้า data
			got = true
			log.Debug().Msg("↪️ JSON looks like ATA; wrapped whole body as data with event=ata.forwarded")
		}

	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		env.Event = c.FormValue("event")
		env.Channel = c.FormValue("channel")
		dataStr := c.FormValue("data")
		env.Data = toRawMessage(dataStr)
		if env.Event != "" || len(env.Data) > 0 {
			got = true
		}

	case strings.HasPrefix(ct, "multipart/"):
		if form, err := c.MultipartForm(); err == nil && form != nil {
			if v := form.Value["event"]; len(v) > 0 {
				env.Event = v[0]
			}
			if v := form.Value["channel"]; len(v) > 0 {
				env.Channel = v[0]
			}
			if v := form.Value["data"]; len(v) > 0 {
				env.Data = toRawMessage(v[0])
			}
			got = (env.Event != "" || len(env.Data) > 0)
		} else {
			log.Warn().Err(err).Msg("⚠️ multipart parse failed")
		}

	case strings.HasPrefix(ct, "text/plain"):
		env.Event = firstNonEmpty(c.Get("X-Event"), "text.forwarded")
		env.Channel = c.Get("X-Channel")
		env.Data = toRawMessage(string(raw))
		got = true

	default:
		// unknown content-type → เดา JSON ก่อน ไม่ได้ก็เก็บ raw เป็น string
		if len(raw) > 0 {
			if json.Unmarshal(raw, &env) == nil && (env.Event != "" || len(env.Data) > 0) {
				got = true
			} else {
				env.Event = firstNonEmpty(c.Get("X-Event"), "raw.forwarded")
				env.Channel = c.Get("X-Channel")
				env.Data = toRawMessage(string(raw))
				got = true
			}
		}
	}

	if !got {
		log.Warn().Str("contentType", ct).Msg("⚠️ No parsable payload for PusherEvent; using raw as data")
		env.Event = firstNonEmpty(c.Get("X-Event"), "raw.forwarded")
		env.Channel = c.Get("X-Channel")
		env.Data = toRawMessage(string(raw))
	}

	// ==== Kafka ====
	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_ATA"),
		"ata.events", // default เฉพาะเส้นนี้
	)

	// ✅ ใช้ key จาก payload (deviceId > id > channelId > sn > device > address > eventDateValue)
	key := deriveKeyFromPusher(env, raw)

	headers := map[string]string{
		"event":       "analytic.pusher.received",
		"schema":      "kai/pusher/1",
		"src_event":   env.Event,
		"src_channel": env.Channel,
		"contentType": ct,
	}

	log.Debug().
		Str("resolved_topic", topic).
		Str("key", key).
		Msg("📤 PusherEvent publishing to Kafka")

	if err := kafka.PublishEventTo(ctx, topic, key, env, headers); err != nil {
		log.Error().Err(err).Str("key", key).Str("topic", topic).Msg("❌ Kafka publish failed")
		return httputil.FailInternal(c, "internal server error")
	}

	log.Debug().
		Str("key", key).Str("topic", topic).
		Str("event", env.Event).Str("channel", env.Channel).
		Msg("✅ Pusher event published")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  true,
		"code":    "SUCCESS",
		"message": "pusher event queued",
		"detail": fiber.Map{
			"topic":   topic,
			"key":     key,
			"event":   env.Event,
			"channel": env.Channel,
		},
	})
}

// -------------------- Helpers --------------------

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// ใช้ข้อมูลใน env.Data มาเป็น key ตามลำดับ field; ไม่เจอค่อย fallback เป็น short hash
func deriveKeyFromPusher(env aimodel.PusherEvent, raw []byte) string {
	// พยายาม parse จาก env.Data ก่อน
	var m map[string]any
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &m) // ไม่เป็น object ก็ข้าม
	}
	// ถ้า env.Data ไม่ใช่ object ลองใช้ raw (เผื่อยังไม่ได้เซ็ต data)
	if len(m) == 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &m)
	}

	candidates := [][]string{
		{"deviceId"}, {"id"}, {"channelId"},
		{"sn"}, {"device"}, {"address"},
		{"eventDateValue"},
	}

	if m != nil {
		if v := deepPickString(m, candidates...); v != "" {
			return v
		}
	}

	// ถ้า data เป็น string (quoted) ก็ใช้เลย
	if len(env.Data) > 0 && env.Data[0] == '"' {
		var s string
		if err := json.Unmarshal(env.Data, &s); err == nil && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}

	// fallback: short hash ของ raw
	sum := sha1.Sum(raw)
	return fmt.Sprintf("%x", sum[:6])
}

// ใช้กับ ATA JSON (non-envelope)
func deriveKafkaKeyFromATA(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		candidates := [][]string{
			{"deviceId"}, {"id"}, {"channelId"},
			{"cameraId"}, {"camera_id"},
			{"eventId"}, {"event_id"},
			{"sn"}, {"device"}, {"address"}, {"eventDateValue"},
			{"details", "deviceId"}, {"details", "device_id"},
			{"details", "cameraId"}, {"details", "camera_id"},
			{"details", "eventId"}, {"details", "event_id"},
			{"source", "deviceId"}, {"source", "device_id"},
			{"camera", "id"}, {"camera", "cameraId"}, {"camera", "camera_id"},
		}
		if v := deepPickString(m, candidates...); v != "" {
			return v
		}
	}
	sum := sha1.Sum(raw)
	return fmt.Sprintf("%x", sum[:6])
}

func deepPickString(m map[string]any, paths ...[]string) string {
	for _, p := range paths {
		if v, ok := deepGetString(m, p...); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// เดิน map ซ้อน ๆ ตาม path (รองรับ case-insensitive key และเลขที่มากับ JSON)
func deepGetString(m map[string]any, path ...string) (string, bool) {
	var cur any = m
	for _, p := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		// exact match ก่อน
		if v, ok := obj[p]; ok {
			cur = v
			continue
		}
		// ลอง case-insensitive
		found := false
		for k, v := range obj {
			if strings.EqualFold(k, p) {
				cur = v
				found = true
				break
			}
		}
		if !found {
			return "", false
		}
	}
	switch v := cur.(type) {
	case string:
		return v, true
	case float64:
		return strconv.FormatInt(int64(v), 10), true
	default:
		return "", false
	}
}

func toRawMessage(s string) json.RawMessage {
	trim := strings.TrimSpace(s)
	if trim == "" {
		return nil
	}
	// ถ้าเป็น JSON อยู่แล้ว (object/array/quoted) ให้ใช้ตามนั้น
	if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") || strings.HasPrefix(trim, "\"") {
		var js json.RawMessage
		if json.Unmarshal([]byte(trim), &js) == nil {
			return js
		}
	}
	// ไม่ใช่ JSON → wrap เป็น string
	b, _ := json.Marshal(trim)
	return b
}
