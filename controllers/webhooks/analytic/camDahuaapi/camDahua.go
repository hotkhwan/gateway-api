package camDahuaapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/repo/stos3minio"
	"klynx/models/aimodel"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func HandleDahua(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/camDahuaapi")
	ctx, span := tracer.Start(ctx, "webhook.HandleDahua")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "camDahuaapi", "HandleDahua")

	// ---- Request meta logs (ช่วยไล่ 400) ----
	ct := string(c.Request().Header.ContentType())
	te := string(c.Request().Header.Peek("Transfer-Encoding"))
	cl := string(c.Request().Header.Peek("Content-Length"))
	ua := string(c.Request().Header.UserAgent())
	ip := c.IP()

	log.Info().
		Str("ip", ip).
		Str("userAgent", ua).
		Str("contentType", ct).
		Str("transferEncoding", te).
		Str("contentLength", cl).
		Msg("🔎 Incoming /dahuahook")

	// 1) ตรวจว่าเป็น multipart จริงไหม
	if !strings.HasPrefix(strings.ToLower(ct), "multipart/") && !strings.HasPrefix(strings.ToLower(ct), "multipart") {
		// บางกล้องตั้งผิด content-type → 415 จะอธิบายชัด
		return httputil.Fail(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			fmt.Sprintf("Expect multipart, got %q", ct))
	}

	// 2) หา boundary ด้วย regex ที่ยืดหยุ่นขึ้น (มี/ไม่มีเว้นวรรค)
	// ตัวอย่าง header: multipart/x-mixed-replace;boundary=myboundary หรือ multipart/form-data; boundary="abc"
	bRe := regexp.MustCompile(`(?i)boundary="?([^";]+)"?`)
	m := bRe.FindStringSubmatch(ct)
	if len(m) < 2 {
		log.Error().Str("contentType", ct).Msg("❌ BOUNDARY_NOT_FOUND")
		return httputil.Fail(c, http.StatusBadRequest, "BOUNDARY_NOT_FOUND", "No boundary in Content-Type")
	}
	boundary := m[1]

	// 3) อ่าน body (ลองทั้ง Body() และ RawBody())
	raw := c.Body()
	if len(raw) == 0 {
		raw = c.Request().Body()
	}
	if len(raw) == 0 {
		// บันทึก header แล้วบอกว่า body ว่าง (อาจเพราะ chunked/stream)
		log.Error().Msg("❌ EMPTY_BODY")
		return httputil.Fail(c, http.StatusBadRequest, "EMPTY_BODY", "Request body is empty (chunked stream?)")
	}

	// 4) เก็บ sample 1KB เพื่อ debug (ไม่ log ทั้งก้อน)
	sample := raw
	if len(sample) > 1024 {
		sample = sample[:1024]
	}
	log.Debug().Msgf("🧪 Body sample (first 1KB):\n%s", string(sample))

	// 5) split โดยรองรับ \r\n และ \n (ลอง 2 รูปแบบ)
	parts := bytes.Split(raw, []byte("\r\n--"+boundary))
	if len(parts) < 2 {
		parts = bytes.Split(raw, []byte("\n--"+boundary))
	}
	// ตัด closing boundary ออก
	// เช่น ... \r\n--boundary--\r\n
	cleaned := make([][]byte, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		// ตัด leading \r\n ที่หลุดติดมา
		p = bytes.TrimPrefix(p, []byte("\r\n"))
		p = bytes.TrimPrefix(p, []byte("\n"))
		// ข้าม closing "--"
		if bytes.HasPrefix(p, []byte("--")) {
			continue
		}
		cleaned = append(cleaned, p)
	}
	parts = cleaned
	log.Debug().Int("parts", len(parts)).Msg("🧩 Multipart parts detected")

	if len(parts) == 0 {
		return httputil.Fail(c, http.StatusBadRequest, "INVALID_MULTIPART", "No parts found after split")
	}

	jsonData := map[string]interface{}{
		"event": "cam.detect",
	}
	imagePaths := []string{}
	imageIndex := 0

	// default datetime/camID fallback
	dateTime := time.Now().Format("20060102T150405")
	camID := "1234"

	// helper: split header/body รองรับทั้ง \r\n\r\n และ \n\n
	splitHeaderBody := func(b []byte) (string, []byte, bool) {
		sep := []byte("\r\n\r\n")
		idx := bytes.Index(b, sep)
		if idx < 0 {
			sep = []byte("\n\n")
			idx = bytes.Index(b, sep)
		}
		if idx < 0 {
			return "", nil, false
		}
		headers := string(b[:idx])
		body := b[idx+len(sep):]
		return headers, body, true
	}

	// เดินทุกพาร์ท ไม่ยึดติดว่า JSON ต้องอยู่ i==1 อีกต่อไป
	for _, part := range parts {
		if len(part) < 10 {
			continue
		}

		headers, body, ok := splitHeaderBody(part)
		if !ok {
			log.Debug().Msg("⚠️ Part without header/body separator")
			continue
		}

		hLower := strings.ToLower(headers)

		// ---- JSON meta (text/plain หรือ application/json) ----
		if strings.Contains(hLower, "content-type: application/json") ||
			strings.Contains(hLower, "content-type: text/plain") {
			// หา {} แรก/สุดท้าย กัน text noise
			start := bytes.IndexByte(body, '{')
			end := bytes.LastIndexByte(body, '}')
			if start >= 0 && end > start {
				if err := json.Unmarshal(body[start:end+1], &jsonData); err != nil {
					log.Error().Err(err).Msg("❌ INVALID_JSON")
					return httputil.Fail(c, http.StatusBadRequest, "INVALID_JSON", "Cannot parse JSON part")
				}
				// enrich time
				if tStr, ok := jsonData["Time"].(string); ok {
					if tParsed, err := time.Parse("2006-01-02 15:04:05", tStr); err == nil {
						dateTime = tParsed.Format("20060102T150405")
					}
				}
				// extract attributes (ย้าย logic เดิม)
				if evs, ok := jsonData["Events"].([]interface{}); ok && len(evs) > 0 {
					if evt, ok := evs[0].(map[string]interface{}); ok {
						if v, ok := evt["Action"]; ok {
							jsonData["action"] = v
						}
						if v, ok := evt["Code"]; ok {
							jsonData["code"] = v
						}
						if data, ok := evt["Data"].(map[string]interface{}); ok {
							if v, ok := data["FaceAttributes"]; ok {
								jsonData["faceAttributes"] = v
							}
							if v, ok := data["HumanAttributes"]; ok {
								jsonData["humanAttributes"] = v
							}
						}
					}
				}
				continue
			}
		}

		// ---- Image parts ----
		if strings.Contains(hLower, "content-type: image/jpeg") || strings.Contains(hLower, "content-type: image/png") {
			imageIndex++
			ext := ".jpg"
			mime := "image/jpeg"
			if strings.Contains(hLower, "content-type: image/png") {
				ext = ".png"
				mime = "image/png"
			}
			img := bytes.TrimRight(body, "\r\n")

			var objectKey string
			switch imageIndex {
			case 1:
				objectKey = fmt.Sprintf("human/%s/%s_full%s", camID, dateTime, ext)
			case 2:
				objectKey = fmt.Sprintf("human/%s/%s_crop%s", camID, dateTime, ext)
			case 3:
				objectKey = fmt.Sprintf("face/%s/%s_full%s", camID, dateTime, ext)
			case 4:
				objectKey = fmt.Sprintf("face/%s/%s_crop%s", camID, dateTime, ext)
			default:
				objectKey = fmt.Sprintf("misc/%s/%s_%d%s", camID, dateTime, imageIndex, ext)
			}

			url, err := stos3minio.Upload(ctx, "cctv", false, objectKey, img, mime)
			if err != nil {
				log.Error().Err(err).Str("objectKey", objectKey).Msg("❌ S3_UPLOAD_FAILED")
				continue
			}
			imagePaths = append(imagePaths, url)
			log.Debug().Str("url", url).Msg("✅ Image uploaded")
		}
	}

	for i, p := range imagePaths {
		jsonData[fmt.Sprintf("imgPath%d", i+1)] = p
	}

	// map -> struct
	var cam aimodel.DuhuaMessage
	tmp, _ := json.Marshal(jsonData)
	if err := json.Unmarshal(tmp, &cam); err != nil {
		log.Error().Err(err).Msg("❌ INVALID_FORMAT (struct mapping)")
		return httputil.FailBadRequest(c, "INVALID_FORMAT", "JSON doesn't match DuhuaMessage")
	}

	if cam.DeviceID == "" {
		if dev, ok := jsonData["DeviceID"].(string); ok {
			cam.DeviceID = dev
		} else {
			cam.DeviceID = camID
		}
	}
	if cam.Timestamp.IsZero() {
		if tStr, ok := jsonData["Time"].(string); ok {
			if tParsed, err := time.Parse("2006-01-02 15:04:05", tStr); err == nil {
				cam.Timestamp = tParsed
			}
		}
	}

	topic := os.Getenv("KAFKA_AI_TOPIC")
	if topic == "" {
		topic = os.Getenv("KAFKA_TOPIC")
		if topic == "" {
			topic = "kai.analytic"
		}
	}
	key := camID
	headers := map[string]string{
		"event":  "cam.dahua.received",
		"schema": "kai/dahua/1",
	}
	if err := kafka.PublishEventTo(ctx, topic, key, cam, headers); err != nil {
		log.Error().Err(err).Msg("❌ KAFKA_ERROR")
		return httputil.FailInternal(c, "internal server error")
	}

	log.Info().Str("deviceId", cam.DeviceID).Int("images", len(imagePaths)).Msg("✅ processed")
	return gmod.SendDahuaResponse(c, len(imagePaths), jsonData)
}

// func HandleDahua(c *fiber.Ctx) error {
// 	ctx := c.UserContext()
// 	tracer := otel.Tracer("klynx/camDahuaapi")
// 	ctx, span := tracer.Start(ctx, "webhook.HandleDahua")
// 	defer span.End()

// 	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
// 		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
// 	}
// 	log := logger.FromCtx(ctx, "camDahuaapi", "HandleDahua")

// 	clientIP := c.IP()
// 	log.Debug().Str("clientIP", clientIP).Msg("📡 Received request")

// 	contentType := c.Get("Content-Type")
// 	log.Debug().Str("contentType", contentType).Msg("🔍 Content-Type header")

// 	// Extract boundary
// 	boundaryRegex := regexp.MustCompile(`boundary="?([^";]+)"?`)
// 	matches := boundaryRegex.FindStringSubmatch(contentType)
// 	if len(matches) < 2 {
// 		log.Debug().Msg("❌ Boundary not found in Content-Type header")
// 		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Boundary not found"})
// 	}
// 	boundary := matches[1]

// 	rawBody := c.Request().Body()
// 	if len(rawBody) == 0 {
// 		log.Debug().Msg("❌ Empty body")
// 		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Empty body"})
// 	}

// 	parts := bytes.Split(rawBody, []byte("--"+boundary))

// 	var jsonData map[string]interface{}
// 	if jsonData == nil {
// 		jsonData = map[string]interface{}{}
// 	}
// 	jsonData["event"] = "cam.detect"
// 	imagePaths := []string{}
// 	imageIndex := 0

// 	// ⏱️ เตรียมค่า dateTime จาก metadata
// 	dateTime := time.Now().Format("20250131T150405")
// 	camID := "1234" // fix ไว้ก่อน

// 	for i, part := range parts {
// 		if len(part) < 10 {
// 			continue
// 		}
// 		headersSplit := bytes.SplitN(part, []byte("\r\n\r\n"), 2)
// 		if len(headersSplit) != 2 {
// 			continue
// 		}
// 		headers := string(headersSplit[0])
// 		body := headersSplit[1]

// 		if strings.Contains(headers, "Content-Type: text/plain") && i == 1 {
// 			start := bytes.IndexByte(body, '{')
// 			end := bytes.LastIndexByte(body, '}')
// 			if start >= 0 && end > start {
// 				if err := json.Unmarshal(body[start:end+1], &jsonData); err != nil {
// 					log.Error().Err(err).Msg("❌ JSON parse error")
// 				}

// 				// enrich metadata
// 				if events, ok := jsonData["Events"].([]interface{}); ok && len(events) > 0 {
// 					if evt, ok := events[0].(map[string]interface{}); ok {
// 						if action, ok := evt["Action"]; ok {
// 							jsonData["action"] = action
// 						}
// 						if code, ok := evt["Code"]; ok {
// 							jsonData["code"] = code
// 						}
// 						if data, ok := evt["Data"].(map[string]interface{}); ok {
// 							if fa, ok := data["FaceAttributes"]; ok {
// 								jsonData["faceAttributes"] = fa
// 							}
// 							if ha, ok := data["HumanAttributes"]; ok {
// 								jsonData["humanAttributes"] = ha
// 							}
// 						}
// 					}
// 				}

// 				// parse datetime จาก metadata ถ้ามี
// 				if tStr, ok := jsonData["Time"].(string); ok {
// 					if tParsed, err := time.Parse("2006-01-02 15:04:05", tStr); err == nil {
// 						dateTime = tParsed.Format("20060102T150405")
// 					}
// 				}
// 			}
// 		} else if strings.Contains(headers, "image/jpeg") || strings.Contains(headers, "image/png") {
// 			imageIndex++
// 			ext := ".jpg"
// 			if strings.Contains(headers, "image/png") {
// 				ext = ".png"
// 			}

// 			// trim CRLF ที่อาจติดมา
// 			body = bytes.TrimRight(body, "\r\n")

// 			// map path ตาม index
// 			var objectKey string
// 			switch imageIndex {
// 			case 1:
// 				objectKey = fmt.Sprintf("human/%s/%s_full%s", camID, dateTime, ext)
// 			case 2:
// 				objectKey = fmt.Sprintf("human/%s/%s_crop%s", camID, dateTime, ext)
// 			case 3:
// 				objectKey = fmt.Sprintf("face/%s/%s_full%s", camID, dateTime, ext)
// 			case 4:
// 				objectKey = fmt.Sprintf("face/%s/%s_crop%s", camID, dateTime, ext)
// 			default:
// 				objectKey = fmt.Sprintf("misc/%s/%s_%d%s", camID, dateTime, imageIndex, ext)
// 			}

// 			url, err := stos3minio.Upload(ctx, "cctv", false, objectKey, body, "image/jpeg")
// 			if err != nil {
// 				log.Error().Err(err).Str("objectKey", objectKey).Msg("❌ Failed to upload image")
// 				continue
// 			}

// 			imagePaths = append(imagePaths, url)
// 			log.Debug().Str("url", url).Msg("✅ Image uploaded to S3")
// 		}
// 	}

// 	for i, path := range imagePaths {
// 		key := fmt.Sprintf("imgPath%d", i+1)
// 		if jsonData == nil {
// 			jsonData = map[string]interface{}{}
// 		}
// 		jsonData[key] = path
// 	}

// 	var cam aimodel.DuhuaMessage
// 	tmp, _ := json.Marshal(jsonData)
// 	if err := json.Unmarshal(tmp, &cam); err != nil {
// 		log.Error().Err(err).Msg("❌ Failed to convert to DuhuaMessage")
// 		return gmod.SendError(c, fiber.StatusBadRequest, "INVALID_FORMAT", "Invalid JSON or structure")
// 	}

// 	// ✅ enrich DeviceID & Timestamp ก่อนส่ง Kafka
// 	if cam.DeviceID == "" {
// 		if dev, ok := jsonData["DeviceID"].(string); ok {
// 			cam.DeviceID = dev
// 		} else {
// 			cam.DeviceID = "1234"
// 		}
// 	}
// 	if cam.Timestamp.IsZero() {
// 		if tStr, ok := jsonData["Time"].(string); ok {
// 			if tParsed, err := time.Parse("2006-01-02 15:04:05", tStr); err == nil {
// 				cam.Timestamp = tParsed
// 			}
// 		}
// 	}

// 	// ✅ Publish to Kafka
// 	topic := os.Getenv("KAFKA_AI_TOPIC")
// 	if topic == "" {
// 		topic = os.Getenv("KAFKA_TOPIC")
// 		if topic == "" {
// 			topic = "kai.analytic"
// 		}
// 	}
// 	key := camID // ใช้ fix camID ก่อน
// 	if key == "" {
// 		key = fmt.Sprintf("img:%d", time.Now().UnixNano())
// 	}
// 	headers := map[string]string{
// 		"event":  "cam.dahua.received",
// 		"schema": "kai/dahua/1",
// 	}

// 	if err := kafka.PublishEventTo(ctx, topic, key, cam, headers); err != nil {
// 		log.Error().Err(err).Msg("❌ Failed to publish to Kafka")
// 		return gmod.SendError(c, fiber.StatusInternalServerError, "KAFKA_ERROR", "Failed to publish to Kafka")
// 	}

// 	log.Debug().Str("deviceId", cam.DeviceID).Int("images", len(imagePaths)).Msg("✅ Dahua message processed successfully")
// 	return gmod.SendDahuaResponse(c, len(imagePaths), jsonData)
// }
