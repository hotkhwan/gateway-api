// internal/services/kaisvc/handlemessage.go
package kaisvc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/kaimod"
	"github.com/hotkhwan/gateway-api/utils/aiutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/google/uuid"
)

func HandleDetect(parent context.Context, msg kaimod.Detection) {
	ctx, end, log := traceutil.StartLite(
		parent,
		"github.com/hotkhwan/gateway-api/kaisvc",        // tracerName
		"grpsvc.HandleDetect", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kaisvc", "HandleDetect",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	detection := &msg.Details.Analyze.Detection

	label := strings.ToUpper(detection.Labels)
	allowedLabels := map[string]bool{
		"VEHICLE": true,
		// "HUMAN": true, // <--- เปิดใช้งานในอนาคต
	}
	if !allowedLabels[label] {
		log.Warn().Str("label", label).Msg("⛔ Skipping unsupported label")
		return
	}

	// ---- ดึงรูปจาก S3 (presigned) ----
	bucket := detection.Bucket
	s3Key := detection.ImageCrop

	expiry := 2 * time.Minute // หรืออ่านจาก config ได้
	urlStr, err := stos3minio.PresignOnce(ctx, bucket, s3Key, expiry)
	if err != nil {
		log.Error().Err(err).Str("key", s3Key).Str("bucket", bucket).Msg("❌ Failed to presign URL")
		return
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(urlStr)
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("❌ HTTP GET failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("url", urlStr).Msg("❌ non-200 from S3")
		return
	}
	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("❌ read body failed")
		return
	}

	// ---- วิเคราะห์ตาม label ----
	var (
		isLpr  bool
		isFace bool
	)

	switch label {
	case "VEHICLE":
		target := aiutil.GetVehicleAIApiURL()
		body, err := aiutil.LprPieApple(imgBytes, "k-analytic.jpg", target)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to call LPR API")
			return
		}

		log.Debug().
			Str("endpoint", target).
			Int("resp_len", len(body)).
			Msg("✅ LPR POST response received")

		if len(body) == 0 || !json.Valid(body) {
			log.Error().Str("body", string(body)).Msg("❌ Invalid or empty JSON from LPR API")
			return
		}

		var src kaimod.SourceLpr
		if err := json.Unmarshal(body, &src); err != nil {
			log.Error().Err(err).RawJSON("body", body).Msg("❌ Failed to unmarshal LPR source result")
			return
		}

		lpr := aiutil.ConvertLpr(src)
		detection.LPR = &lpr

		isLpr = lpr.LicensePlate != nil
		isFace = false
		detection.LprFlag = &isLpr
		detection.FaceFlag = &isFace

		if detection.LPR == nil || detection.LPR.LicensePlate == nil {
			log.Warn().Msg("⚠️ No license plate detected from LPR API")
		}
	}

	// ---- สร้างและยิง Kafka Event ----
	now := time.Now().UTC()

	evt := kaimod.AIResult{
		ID:          uuid.NewString(),             // ไอดีของ event payload
		Event:       "detection.created",          // ชื่อ event
		Time:        now.Format(time.RFC3339Nano), // เวลาของ event
		Rev:         0,                            // เริ่มที่ 0 (ถ้ามีการอัปเดต event schema ให้เพิ่มเลขนี้)
		Details:     msg.Details,                  // ใส่ผล LPR/Face ใน details แล้ว
		ProcessedAt: now.Format(time.RFC3339),     // เวลา process
		EventID:     uuid.New().String(),          // สำหรับอ้างอิงภายนอก/ดีบัก
		Lpr:         isLpr,
		Face:        isFace,
	}

	// เลือก partition key ที่ “คงที่” ต่อเหตุการณ์ประเภทเดียวกัน
	// แนะนำ: ถ้ามี CameraID/DeviceID ใช้ตัวนั้น; ไม่มีก็ fallback เป็น s3Key
	kKey := s3Key
	if kKey == "" {
		kKey = uuid.NewString()
	}

	// topic จาก ENV (ถ้าไม่ตั้ง ไปลงค่าเริ่มต้น)
	topic := os.Getenv("KAFKA_AI_TOPIC")
	if topic == "" {
		topic = "kai.analytic"
	}

	// headers: ใส่เฉพาะที่อยาก override; ที่เหลือ wrapper เติมให้เองจาก ENV (EVENT_SOURCE, trace_id, idempotency_key, rev จาก evt ฯลฯ)
	headers := map[string]string{
		"schema": "kai/analytic/1", // family + เวอร์ชันสคีมา
		// "source": "kaisvc",       // ปล่อยให้ EVENT_SOURCE จัดการก็ได้
	}

	// ใช้ ctx แบบ background ก็ได้ เพราะ wrapper จะเติม default timeout (เช่น 5s) ถ้า ctx ไม่มี deadline
	if err := kafka.PublishEventTo(ctx, topic, kKey, evt, headers); err != nil {
		log.Error().Err(err).Str("id", evt.ID).Str("topic", topic).Msg("❌ failed to emit AI result")
		return
	}

	log.Debug().
		Str("topic", topic).
		Str("eventID", evt.EventID).
		Str("key", kKey).
		Msg("✅ AI result published to Kafka")
}
