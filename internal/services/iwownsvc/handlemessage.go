// internal/services/iwownsvc/handlemessage.go
package iwownsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/iwown/iwownParser"
	pb "github.com/hotkhwan/gateway-api/internal/iwown/protobuf"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/iwownmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

const (
	collectionIwownEvents = "iwown_events"
)

// --------- Common Mongo Document ---------
type EventDoc struct {
	DeviceID  string `bson:"deviceId,omitempty" json:"deviceId,omitempty"`
	EventType string `bson:"eventType" json:"eventType"`
	Kind      string `bson:"kind,omitempty" json:"kind,omitempty"`
	Index     int    `bson:"index,omitempty" json:"index,omitempty"`
	Opt       uint16 `bson:"opt,omitempty" json:"opt,omitempty"`

	Status    string `bson:"status,omitempty" json:"status,omitempty"`
	SleepDate string `bson:"sleepDate,omitempty" json:"sleepDate,omitempty"`

	Parsed   map[string]any `bson:"parsed,omitempty" json:"parsed,omitempty"`
	Computed map[string]any `bson:"computed,omitempty" json:"computed,omitempty"`

	RawJSON   map[string]any `bson:"rawJson,omitempty" json:"rawJson,omitempty"`
	EventTime *time.Time     `bson:"eventTime,omitempty" json:"eventTime,omitempty"`
	CreatedAt time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time      `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

// -------------------- Binary Frames --------------------

func HandlePBFrame(parent context.Context, frame any) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"github.com/hotkhwan/gateway-api/iwownsvc",
		"iwownsvc.HandlePBFrame",
		"iwownsvc", "HandlePBFrame",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log.Debug().Msg("▶️ HandlePBFrame start")
	return handleBinaryFrame(ctx, log, "pb_frame", frame)
}

func HandleAlarmFrame(parent context.Context, frame any) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"github.com/hotkhwan/gateway-api/iwownsvc",
		"iwownsvc.HandleAlarmFrame",
		"iwownsvc", "HandleAlarmFrame",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log.Debug().Msg("▶️ HandleAlarmFrame start")
	return handleBinaryFrame(ctx, log, "alarm_frame", frame)
}

func handleBinaryFrame(ctx context.Context, log zerolog.Logger, eventType string, frameAny any) error {
	now := time.Now().UTC()

	log.Debug().
		Str("eventType", eventType).
		Str("frameType", fmt.Sprintf("%T", frameAny)).
		Msg("📥 iwown binary frame received")

	frame, payloadBytes, err := normalizeBinaryFrame(frameAny)
	if err != nil {
		log.Error().Err(err).Str("eventType", eventType).Msg("❌ normalizeBinaryFrame failed")
		return err
	}
	if len(payloadBytes) == 0 {
		log.Error().Str("deviceId", frame.DeviceID).Uint16("opt", frame.Opt).Msg("❌ empty payload after decode")
		return fmt.Errorf("empty payload after decode")
	}

	log.Debug().
		Str("deviceId", frame.DeviceID).
		Uint16("opt", frame.Opt).
		Int("payloadLen", len(payloadBytes)).
		Msg("✅ payload decoded")

	parsed := map[string]any{
		"kind":       frame.Kind,
		"opt":        frame.Opt,
		"payloadLen": len(payloadBytes),
	}
	computed := map[string]any{}

	switch frame.Opt {

	case 0x80:
		// ✅ สำคัญ: 0x80 จาก vendor จริง ๆ คือ HisData wrapper (มีหลาย subtype: health/gnss/spo2/temp/medic/...)
		// เราพยายาม Unmarshal เป็น pb.HisData ก่อนเสมอ
		var his pb.HisData
		if err := proto.Unmarshal(payloadBytes, &his); err == nil {
			switch {
			case his.GetHealth() != nil:
				// ยังไม่ทำ MapHisHealth ก็เก็บ raw ก่อน (กัน compile error)
				parsed["his_health_raw"] = his.GetHealth()

			case his.GetGnss() != nil:
				parsed["his_gnss"] = iwownParser.MapHisDataGNSS(his.GetGnss())

			case his.GetSpo2() != nil:
				parsed["his_spo2"] = iwownParser.MapHisDataSpo2(his.GetSpo2())

			case his.GetTemp() != nil:
				parsed["his_temp"] = iwownParser.MapHisDataTemperature(his.GetTemp())

			case his.GetMedic() != nil:
				parsed["his_medic"] = iwownParser.MapHisDataMedic(his.GetMedic())

			default:
				// บางครั้ง vendor ส่ง 0x80 แต่ไม่มี oneof ที่เรารู้จัก (หรือ proto file ไม่ match version)
				// -> fallback เป็น history80 (format ที่คุณแกะเอง)
				h80, herr := iwownParser.ParseHistory80(payloadBytes)
				if herr != nil {
					parsed["error"] = herr.Error()
				} else {
					parsed["history80"] = h80
				}
			}
		} else {
			// Unmarshal HisData ไม่ผ่าน -> fallback history80
			h80, herr := iwownParser.ParseHistory80(payloadBytes)
			if herr != nil {
				parsed["error"] = herr.Error()
			} else {
				parsed["history80"] = h80
			}
		}

	case 0x0A, 0x10: // OM0
		out, err := iwownParser.ParseOM0Report(payloadBytes, int(frame.Opt))
		if err != nil {
			log.Error().Err(err).Msg("❌ parse OM0 failed")
			parsed["error"] = err.Error()
			break
		}
		b, _ := json.Marshal(out)
		_ = json.Unmarshal(b, &parsed)
		log.Info().Msg("✅ OM0 parsed")

	case 0x12: // Alarm
		out, err := iwownParser.ParseAlarmFrame(payloadBytes, int(frame.Opt))
		if err != nil {
			log.Error().Err(err).Msg("❌ parse Alarm failed")
			parsed["error"] = err.Error()
			break
		}
		b, _ := json.Marshal(out)
		_ = json.Unmarshal(b, &parsed)
		log.Info().Msg("✅ Alarm parsed")

	default:
		log.Warn().Uint16("opt", frame.Opt).Msg("⚠️ unknown opt")
		parsed["note"] = "unknown opt"
	}

	doc := EventDoc{
		DeviceID:  frame.DeviceID,
		EventType: eventType,
		Kind:      frame.Kind,
		Index:     frame.Index,
		Opt:       frame.Opt,
		Parsed:    parsed,
		Computed:  computed,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := stomongo.InsertOne(ctx, collectionIwownEvents, doc); err != nil {
		log.Error().Err(err).Str("collection", collectionIwownEvents).Msg("❌ insert iwown event failed")
		return err
	}

	log.Debug().Str("deviceId", frame.DeviceID).Uint16("opt", frame.Opt).Msg("✅ iwown event inserted")
	return nil
}

// -------------------- JSON Events (CallLog/Info/Status) --------------------

func HandleCallLog(ctx context.Context, dataAny any) error {
	return insertJSONEvent(ctx, "call_log", dataAny)
}
func HandleDeviceInfo(ctx context.Context, dataAny any) error {
	return insertJSONEvent(ctx, "device_info", dataAny)
}
func HandleStatus(ctx context.Context, dataAny any) error {
	return insertJSONEvent(ctx, "status", dataAny)
}

func insertJSONEvent(ctx context.Context, eventType string, dataAny any) error {
	now := time.Now().UTC()

	var raw map[string]any
	b, err := json.Marshal(dataAny)
	if err != nil {
		return fmt.Errorf("marshal dataAny failed: %w", err)
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("unmarshal to map failed: %w", err)
	}

	doc := EventDoc{
		EventType: eventType,
		RawJSON:   raw,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// deviceid variants
	if v, ok := raw["deviceid"].(string); ok && v != "" {
		doc.DeviceID = v
	}
	if v, ok := raw["DeviceId"].(string); ok && v != "" && doc.DeviceID == "" {
		doc.DeviceID = v
	}
	if v, ok := raw["deviceId"].(string); ok && v != "" && doc.DeviceID == "" {
		doc.DeviceID = v
	}

	// status
	if v, ok := raw["status"].(string); ok && v != "" {
		doc.Status = v
	}
	if v, ok := raw["Status"].(string); ok && v != "" && doc.Status == "" {
		doc.Status = v
	}

	// sleep_date
	if v, ok := raw["sleep_date"].(string); ok && v != "" {
		doc.SleepDate = v
	}

	// eventTime "YYYY-MM-DD HH:mm:ss"
	if v, ok := raw["eventTime"].(string); ok && v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.UTC); err == nil {
			doc.EventTime = &t
		}
	}
	if v, ok := raw["EventTime"].(string); ok && v != "" && doc.EventTime == nil {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.UTC); err == nil {
			doc.EventTime = &t
		}
	}

	if _, err := stomongo.InsertOne(ctx, collectionIwownEvents, doc); err != nil {
		return fmt.Errorf("insert iwown json event failed: %w", err)
	}
	return nil
}

// -------------------- Deterministic payload normalization --------------------

// normalizeBinaryFrame: กัน decode ซ้ำ + ได้ผล deterministic
func normalizeBinaryFrame(frameAny any) (iwownmod.IwownBinaryFrameEvent, []byte, error) {
	var frame iwownmod.IwownBinaryFrameEvent

	decodeB64 := func(s string) ([]byte, error) {
		if s == "" {
			return nil, fmt.Errorf("empty base64 payload")
		}
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		if b, err := base64.URLEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		return nil, fmt.Errorf("invalid base64 payload")
	}

	// 1) typed event => ใช้ Payload ตรง ๆ ห้าม decode ซ้ำ
	switch v := frameAny.(type) {
	case iwownmod.IwownBinaryFrameEvent:
		frame = v
		if len(frame.Payload) == 0 {
			return frame, nil, fmt.Errorf("empty payload")
		}
		return frame, frame.Payload, nil

	case *iwownmod.IwownBinaryFrameEvent:
		if v == nil {
			return frame, nil, fmt.Errorf("nil frame")
		}
		frame = *v
		if len(frame.Payload) == 0 {
			return frame, nil, fmt.Errorf("empty payload")
		}
		return frame, frame.Payload, nil
	}

	// 2) map => อย่า json roundtrip ก่อน (มันชอบเปลี่ยน []byte เป็น base64 string)
	if m, ok := frameAny.(map[string]any); ok {
		// fill struct fields (best effort)
		b, _ := json.Marshal(m)
		_ = json.Unmarshal(b, &frame)

		// payload priority:
		//   a) payload is []byte
		//   b) payload is string(base64)
		//   c) frame.Payload (if json unmarshal already put it)
		if p, ok := pickAny(m, "payload", "Payload"); ok {
			switch pv := p.(type) {
			case []byte:
				if len(pv) > 0 {
					return frame, pv, nil
				}
			case string:
				decoded, err := decodeB64(pv)
				if err != nil {
					return frame, nil, err
				}
				return frame, decoded, nil
			}
		}

		if len(frame.Payload) > 0 {
			return frame, frame.Payload, nil
		}
		return frame, nil, fmt.Errorf("empty payload")
	}

	// 3) fallback: json roundtrip
	b, err := json.Marshal(frameAny)
	if err != nil {
		return frame, nil, err
	}
	if err := json.Unmarshal(b, &frame); err != nil {
		return frame, nil, err
	}
	if len(frame.Payload) > 0 {
		return frame, frame.Payload, nil
	}

	// final fallback: extract payload string then decode
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err == nil {
		if p, ok := pickAny(raw, "payload", "Payload"); ok {
			if s, ok := p.(string); ok {
				decoded, derr := decodeB64(s)
				if derr == nil && len(decoded) > 0 {
					return frame, decoded, nil
				}
			}
		}
	}

	return frame, nil, fmt.Errorf("empty payload after decode")
}

func pickAny(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}
