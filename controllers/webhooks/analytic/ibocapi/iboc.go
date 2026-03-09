// controllers/webhooks/analytic/ibocapi/iboc.go
package ibocapi

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/models/aimodel"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

func HandleIboc(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.webhooks.ibocapi", "HandleIboc", "webhooks", "HandleIboc")
	defer end()

	clientIP := c.IP()
	log.Info().Str("clientIP", clientIP).Msg("📡 IBOC: Incoming request")

	contentType := c.Get("Content-Type")
	log.Info().Str("contentType", contentType).Msg("🔍 Content-Type")

	rawBody := c.Body()

	// ✅ Pretty-print log JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawBody, "", "  "); err != nil {
		log.Error().Err(err).Msg("❌ Invalid JSON (indent error)")
		return httputil.Fail(c, fiber.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
	}
	log.Debug().Msgf("📝 JSON Payload:\n%s", prettyJSON.String())

	// ✅ Parse JSON เป็น struct
	var cam aimodel.IbocMessage
	if err := json.Unmarshal(rawBody, &cam); err != nil {
		log.Error().Err(err).Msg("❌ Failed to parse to IbocMessage")
		return httputil.FailBadRequest(c, "Invalid JSON", "Invalid IBOC message format")
		// return httputil.Fail(c, fiber.StatusBadRequest, "INVALID_FORMAT", "Invalid IBOC message format")
	}

	// ✅ สร้าง Kafka key แบบยืดหยุ่นจาก JSON ดิบ (ไม่ผูกกับ struct fields)
	key := deriveKafkaKeyFromIBOC(rawBody)

	// ✅ Publish to Kafka
	topic := os.Getenv("KAFKA_AI_TOPIC")
	if topic == "" {
		topic = os.Getenv("KAFKA_TOPIC")
		if topic == "" {
			topic = "kai.analytic"
		}
	}

	headers := map[string]string{
		"event":  "analytic.iboc.received",
		"schema": "kai/iboc/1",
	}

	if err := kafka.PublishEventTo(ctx, topic, key, cam, headers); err != nil {
		log.Error().Err(err).Str("key", key).Str("topic", topic).Msg("❌ Failed to send to Kafka")
		return httputil.FailInternal(c, "internal server error")
		// return httputil.Fail(c, fiber.StatusInternalServerError, "KAFKA_ERROR", "Failed to publish to Kafka")
	}

	// ✅ Log สรุปข้อมูลสำคัญ
	log.Info().
		Str("key", key).
		Msg("✅ IBOC message processed successfully")
	return gmod.SendIbocResponse(c, cam)
}

// ----------------- helpers -----------------

// พยายามดึง key จากหลาย path/case; ไม่เจอให้ fallback เป็น hash ของ payload
func deriveKafkaKeyFromIBOC(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		candidates := [][]string{
			{"deviceId"}, {"device_id"},
			{"cameraId"}, {"camera_id"},
			{"eventId"}, {"event_id"},
			{"details", "deviceId"}, {"details", "device_id"},
			{"details", "cameraId"}, {"details", "camera_id"},
			{"details", "eventId"}, {"details", "event_id"},
			{"source", "deviceId"}, {"source", "device_id"},
			{"camera", "id"}, {"camera", "cameraId"}, {"camera", "camera_id"},
		}
		for _, path := range candidates {
			if v, ok := deepGetString(m, path...); ok && v != "" {
				return v
			}
		}
	}
	sum := sha1.Sum(raw)
	return fmt.Sprintf("iboc:%x", sum)
}

// เดิน map ซ้อน ๆ ตาม path (รองรับ case-insensitive key และเลขที่มากับ JSON)
func deepGetString(m map[string]any, path ...string) (string, bool) {
	var cur any = m
	for _, p := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		// ตรงตัวก่อน
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
