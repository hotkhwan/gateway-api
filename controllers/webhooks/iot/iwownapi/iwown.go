// controllers/webhooks/iot/iwownapi/iwown.go
package iwownapi

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/models/iwownmod"
	"klynx/utils/traceutil"
)

func HandleIwownPbUpload(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwownapi")
	ctx, span := tracer.Start(ctx, "webhook.IwownPbUpload")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwownapi", "pb-upload")

	payload := c.Body()
	if len(payload) == 0 {
		log.Warn().Msg("⚠️ pb upload empty body")
		return c.Status(fiber.StatusOK).Send([]byte{0x02})
	}
	if len(payload) < 23 {
		log.Warn().Int("len", len(payload)).Msg("⚠️ pb upload data length below 23")
		return c.Status(fiber.StatusOK).Send([]byte{0x02})
	}

	deviceBytes := make([]byte, 15)
	copy(deviceBytes, payload[:15])
	device := strings.TrimSpace(string(deviceBytes))

	previewLen := 32
	if len(payload) < previewLen {
		previewLen = len(payload)
	}
	log.Debug().
		Int("len", len(payload)).
		Str("device", device).
		Str("preview_hex", hex.EncodeToString(payload[:previewLen])).
		Msg("📦 iwown PB upload received")

	// ✅ ใช้ topic เดียว
	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_IWOWN"),
		"kwatch4g.iwown",
	)

	startPos := 15
	index := 0

	for {
		if len(payload) < startPos+8 {
			log.Warn().Int("startPos", startPos).Msg("⚠️ pb data length below header+meta")
			return c.Status(fiber.StatusOK).Send([]byte{0x02})
		}

		prefix0 := payload[startPos]
		prefix1 := payload[startPos+1]
		if prefix0 != 0x44 || prefix1 != 0x54 { // 'D','T'
			log.Warn().
				Int("startPos", startPos).
				Uint8("b0", prefix0).
				Uint8("b1", prefix1).
				Msg("⚠️ invalid pb data header")
			return c.Status(fiber.StatusOK).Send([]byte{0x03})
		}

		lenBytes := payload[startPos+2 : startPos+4]
		length := int(lenBytes[1])*0x100 + int(lenBytes[0])

		if len(payload) < startPos+8+length {
			log.Warn().
				Int("startPos", startPos).
				Int("length", length).
				Msg("⚠️ pb data length below header+length")
			return c.Status(fiber.StatusOK).Send([]byte{0x02})
		}

		optBytes := payload[startPos+6 : startPos+8]
		opt := binary.LittleEndian.Uint16(optBytes)

		pbPayload := payload[startPos+8 : startPos+8+length]

		ev := iwownmod.IwownBinaryFrameEvent{
			Kind:     "pb",
			Index:    index,
			DeviceID: device,
			Opt:      opt,
			Payload:  pbPayload,
		}

		key := device
		if strings.TrimSpace(key) == "" {
			key = deriveKeyForIwownBinary(pbPayload)
		}

		headers := map[string]string{
			"event": "kwatch4g.pb", // ✅ ใช้ header แยก type
			"kind":  "pb",
		}

		if err := kafka.PublishEventTo(ctx, topic, key, ev, headers); err != nil {
			log.Error().Err(err).
				Str("topic", topic).
				Str("key", key).
				Int("index", index).
				Msg("❌ Kafka publish failed for iwown PB frame")
			return c.Status(fiber.StatusOK).Send([]byte{0x01})
		}

		log.Debug().
			Str("topic", topic).
			Str("key", key).
			Int("index", index).
			Uint16("opt", opt).
			Msg("✅ iwown PB frame published")

		index++
		startPos += 8 + length
		if len(payload) == startPos {
			break
		}
	}

	return c.Status(fiber.StatusOK).Send([]byte{0x00})
}

func HandleIwownAlarmUpload(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwown")
	ctx, span := tracer.Start(ctx, "webhook.IwownAlarmUpload")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwown", "alarm-upload")

	payload := c.Body()
	if len(payload) == 0 {
		log.Warn().Msg("⚠️ alarm upload empty body")
		return c.Status(fiber.StatusOK).Send([]byte{0x02})
	}
	if len(payload) < 23 {
		log.Warn().Int("len", len(payload)).Msg("⚠️ alarm data length below 23")
		return c.Status(fiber.StatusOK).Send([]byte{0x02})
	}

	deviceBytes := make([]byte, 15)
	copy(deviceBytes, payload[:15])
	device := strings.TrimSpace(string(deviceBytes))

	previewLen := 32
	if len(payload) < previewLen {
		previewLen = len(payload)
	}
	log.Debug().
		Int("len", len(payload)).
		Str("device", device).
		Str("preview_hex", hex.EncodeToString(payload[:previewLen])).
		Msg("📦 iwown Alarm upload received")

	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_IWOWN"),
		"kwatch4g.iwown",
	)

	startPos := 15
	index := 0

	for {
		if len(payload) < startPos+8 {
			log.Warn().Int("startPos", startPos).Msg("⚠️ alarm data length below header+meta")
			return c.Status(fiber.StatusOK).Send([]byte{0x02})
		}

		prefix0 := payload[startPos]
		prefix1 := payload[startPos+1]
		if prefix0 != 0x44 || prefix1 != 0x54 {
			log.Warn().
				Int("startPos", startPos).
				Uint8("b0", prefix0).
				Uint8("b1", prefix1).
				Msg("⚠️ invalid alarm data header")
			return c.Status(fiber.StatusOK).Send([]byte{0x03})
		}

		lenBytes := payload[startPos+2 : startPos+4]
		length := int(lenBytes[1])*0x100 + int(lenBytes[0])

		if len(payload) < startPos+8+length {
			log.Warn().
				Int("startPos", startPos).
				Int("length", length).
				Msg("⚠️ alarm data length below header+length")
			return c.Status(fiber.StatusOK).Send([]byte{0x02})
		}

		optBytes := payload[startPos+6 : startPos+8]
		opt := binary.LittleEndian.Uint16(optBytes)

		pbPayload := payload[startPos+8 : startPos+8+length]

		ev := iwownmod.IwownBinaryFrameEvent{
			Kind:     "alarm",
			Index:    index,
			DeviceID: device,
			Opt:      opt,
			Payload:  pbPayload,
		}

		key := device
		if strings.TrimSpace(key) == "" {
			key = deriveKeyForIwownBinary(pbPayload)
		}

		headers := map[string]string{
			"event": "kwatch4g.alarm",
			"kind":  "alarm",
		}

		if err := kafka.PublishEventTo(ctx, topic, key, ev, headers); err != nil {
			log.Error().Err(err).
				Str("topic", topic).
				Str("key", key).
				Int("index", index).
				Msg("❌ Kafka publish failed for iwown Alarm frame")
			return c.Status(fiber.StatusOK).Send([]byte{0x01})
		}

		log.Debug().
			Str("topic", topic).
			Str("key", key).
			Int("index", index).
			Uint16("opt", opt).
			Msg("✅ iwown Alarm frame published")

		index++
		startPos += 8 + length
		if len(payload) == startPos {
			break
		}
	}

	return c.Status(fiber.StatusOK).Send([]byte{0x00})
}

func HandleIwownCallLogUpload(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwown")
	ctx, span := tracer.Start(ctx, "webhook.IwownCallLogUpload")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwown", "calllog-upload")

	var req iwownmod.IwownDeviceCallLogs
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ parse call_log payload failed")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	log.Debug().
		Str("deviceid", req.Deviceid).
		Int("normal_logs", len(req.NormalCallLogs)).
		Int("sos_logs", len(req.Sos)).
		Msg("📞 iwown call_log upload")

	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_IWOWN"),
		"kwatch4g.iwown",
	)

	key := strings.TrimSpace(req.Deviceid)
	if key == "" {
		key = deriveKeyForIwownBinary(c.Body())
	}

	headers := map[string]string{
		"event": "kwatch4g.calllog",
	}

	if err := kafka.PublishEventTo(ctx, topic, key, req, headers); err != nil {
		log.Error().Err(err).
			Str("topic", topic).
			Str("key", key).
			Msg("❌ Kafka publish failed for call_log")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 0})
}

func HandleIwownDeviceInfoUpload(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwown")
	ctx, span := tracer.Start(ctx, "webhook.IwownDeviceInfoUpload")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwown", "deviceinfo-upload")

	var req iwownmod.IwownDeviceInfo
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ parse deviceinfo payload failed")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	log.Debug().
		Str("deviceid", req.Deviceid).
		Str("model", req.Model).
		Str("version", req.Version).
		Msg("📟 iwown deviceinfo upload")

	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_IWOWN"),
		"kwatch4g.iwown",
	)

	key := strings.TrimSpace(req.Deviceid)
	if key == "" {
		key = deriveKeyForIwownBinary(c.Body())
	}

	headers := map[string]string{
		"event": "kwatch4g.deviceinfo",
	}

	if err := kafka.PublishEventTo(ctx, topic, key, req, headers); err != nil {
		log.Error().Err(err).
			Str("topic", topic).
			Str("key", key).
			Msg("❌ Kafka publish failed for deviceinfo")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 0})
}

func HandleIwownStatusNotify(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwown")
	ctx, span := tracer.Start(ctx, "webhook.IwownStatusNotify")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwown", "status-notify")

	var req iwownmod.IwownDeviceStatus
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ parse status notify payload failed")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	log.Info().
		Str("deviceId", req.DeviceId).
		Str("eventTime", req.EventTime).
		Str("status", req.Status).
		Msg("📡 iwown device status change")

	topic := firstNonEmpty(
		os.Getenv("KAFKA_TOPIC_IWOWN"),
		"kwatch4g.iwown",
	)

	key := strings.TrimSpace(req.DeviceId)
	if key == "" {
		key = deriveKeyForIwownBinary(c.Body())
	}

	headers := map[string]string{
		"event": "kwatch4g.status",
	}

	if err := kafka.PublishEventTo(ctx, topic, key, req, headers); err != nil {
		log.Error().Err(err).
			Str("topic", topic).
			Str("key", key).
			Msg("❌ Kafka publish failed for status notify")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 0})
}

func HandleIwownHealthSleep(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/iwown")
	ctx, span := tracer.Start(ctx, "webhook.IwownHealthSleep")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "iwown", "health-sleep")

	deviceid := c.Query("deviceid")
	sleepDate := c.Query("sleep_date")

	if strings.TrimSpace(deviceid) == "" || !iwownIsValidDate(sleepDate) {
		log.Warn().
			Str("deviceid", deviceid).
			Str("sleep_date", sleepDate).
			Msg("⚠️ invalid health sleep query")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	prevDay, err := iwownGetPreviousDay(sleepDate)
	if err != nil {
		log.Error().Err(err).Msg("❌ parse previous day failed")
		return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseCode{ReturnCode: 10002})
	}

	res := &iwownmod.IwownSleepResult{
		Deviceid:     deviceid,
		SleepDate:    sleepDate,
		StartTime:    prevDay + " 23:15:00",
		EndTime:      sleepDate + " 07:00:00",
		DeepSleep:    85,
		LightSleep:   300,
		WeakSleep:    30,
		EyemoveSleep: 50,
		Score:        80,
		OsahsRisk:    0,
		Spo2Score:    0,
		SleepHr:      60,
	}

	log.Debug().
		Str("deviceid", deviceid).
		Str("sleep_date", sleepDate).
		Msg("💤 iwown health sleep mock result returned")

	return c.Status(fiber.StatusOK).JSON(iwownmod.IwownResponseObject{
		ReturnCode: 0,
		Data:       res,
	})
}

// ---------- Helpers ----------

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func deriveKeyForIwownBinary(raw []byte) string {
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:6])
}

func iwownIsValidDate(dateStr string) bool {
	_, err := time.Parse("2006-01-02", dateStr)
	return err == nil
}

func iwownGetPreviousDay(dateStr string) (string, error) {
	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", err
	}
	prev := parsed.AddDate(0, 0, -1)
	return prev.Format("2006-01-02"), nil
}
