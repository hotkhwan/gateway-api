// internal/mqtt/kcontrolmsg/handler.go
package kcontrolmsg

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func MessageHandler(client mqtt.Client, msg mqtt.Message) {
	log := logger.WithMeta("kcontrol", "internal-mqtt-MessageHandler")

	// ✅NO retained message alarm topic ตอน subscribe/reconnect
	if msg.Retained() && msg.Topic() == "kcontrol.alarms" {
	log.Debug().Str("topic", msg.Topic()).Msg("🟡 Retained alarms, skip")
	return
	}


	topic := msg.Topic()
	payload := msg.Payload()
	ctx := context.Background()

	// ข้าม retained-clear
	if len(payload) == 0 || string(payload) == `""` {
		log.Debug().Str("topic", topic).Msg("🟡 Empty MQTT payload (retained cleared), skip")
		return
	}

	// --------- Parse payload → map ----------
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		log.Warn().Err(err).Str("topic", topic).RawJSON("payload", payload).Msg("⚠️ Invalid MQTT payload")
		return
	}

	// hwId (ต้องมีเพื่อผูก device)
	hwID := strOrEmpty(m["hwId"])
	if hwID == "" {
		if mac := strOrEmpty(m["mac"]); mac != "" {
			hwID = mac
		}
	}
	if hwID == "" {
		log.Warn().RawJSON("payload", payload).Msg("⚠️ Missing hwId — cannot upsert device, only forward to Kafka")
	}

	// ---------- kind จาก topic ----------
	kind := "unknown"
	switch topic {
	case "kcontrol.sensor":
		kind = "sensor"
	case "kcontrol.alarms":
		kind = "alarms"
	case "kcontrol.health":
		kind = "health"
	}

	// ---------- timestamp (firedAt > startedAt > timestamp > now) ----------
	ts := time.Now()
	if v, ok := toInt64(m["firedAt"]); ok && v > 0 {
		ts = time.Unix(v, 0)
	} else if v, ok := toInt64(m["startedAt"]); ok && v > 0 {
		ts = time.Unix(v, 0)
	} else if v, ok := toInt64(m["timestamp"]); ok && v > 0 {
		ts = time.Unix(v, 0)
	} else if s, ok := m["timestamp"].(string); ok && strings.TrimSpace(s) != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			ts = parsed
		}
	}

	// ---------- HEALTH → upsert MongoDB kcontrol (ไม่มี ratelimit/dedup แล้ว) ----------
	if kind == "health" && hwID != "" && config.MongoClient != nil {
		if err := upsertDeviceFromHealth(ctx, hwID, payload); err != nil {
			log.Error().Err(err).Str("hwId", hwID).Msg("❌ Upsert device from health failed")
		}
	}

	// ---------- เช็ค approved ก่อน forward เข้า Kafka ----------
	if hwID != "" && config.MongoClient != nil {
		approved, err := isDeviceApproved(ctx, hwID)
		if err != nil {
			log.Error().Err(err).Str("hwId", hwID).Msg("❌ Failed to check device approved")
		}
		if !approved {
			log.Debug().
				Str("hwId", hwID).
				Str("topic", topic).
				Msg("⛔ Unapproved device, skip forwarding to Kafka")
			return
		}
	}

	// ---------- สร้าง envelope แล้ว Forward ไป Kafka ----------
	out := map[string]any{
		"topic":     topic,
		"hwId":      hwID,
		"timestamp": ts.UTC().Format(time.RFC3339),
		"kind":      kind,
		"bridge":    os.Getenv("POD_NAME"),
	}
	for k, v := range m {
		switch k {
		case "topic", "hwId", "timestamp", "kind":
			continue
		default:
			out[k] = v
		}
	}

	key := hwID
	if key == "" {
		h := sha1.Sum(payload)
		key = fmt.Sprintf("%x", h)
	}
	headers := map[string]string{
		"event":  "kcontrol.event.received",
		"schema": "kcontrol/envelope/3",
		"kind":   kind,
		"bridge": os.Getenv("POD_NAME"),
	}
	if err := kafka.PublishEventTo(ctx, topic, key, out, headers); err != nil {
		log.Error().Err(err).Str("topic", topic).Msg("❌ Kafka publish failed")
		return
	}
	log.Debug().
		Str("hwId", hwID).
		Str("topic", topic).
		Str("kind", kind).
		Msg("✅ Event forwarded to Kafka")
}

/* ===== Mongo upsert จาก HEALTH ===== */

func upsertDeviceFromHealth(ctx context.Context, hwID string, payload []byte) error {
	log := logger.WithMeta("kcontrol", "internal-mqtt-MessageHandler-upsertDeviceFromHealth")

	var msg kctrlmod.HealthMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		log.Warn().Err(err).Str("hwId", hwID).Msg("⚠️ Unmarshal HealthMessage failed")
		return err
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol")
	now := time.Now().UTC()

	// แปลง interval → spec
	in1 := toIntervalSpec(msg.IntervalIN1, "interval_IN_1", "SENSOR1")
	in2 := toIntervalSpec(msg.IntervalIN2, "interval_IN_2", "SENSOR2")
	in3 := toIntervalSpec(msg.IntervalIN3, "interval_IN_3", "SENSOR3")
	in4 := toIntervalSpec(msg.IntervalIN4, "interval_IN_4", "SENSOR4")

	sensors := make([]kctrlmod.IntervalSpec, 0, 4)
	for _, in := range []*kctrlmod.IntervalSpec{in1, in2, in3, in4} {
		if in != nil {
			sensors = append(sensors, *in)
		}
	}

	filter := bson.M{"hwId": hwID}

	// ❗ ตั้งค่าที่อัปเดตทุก health (ทั้ง create + update)
	set := bson.M{
		"lastSeen":       now,
		"dateTimeUpdate": now,

		"fwVersion": msg.FwVersion,
		"fwBuild":   msg.FwBuild,
		"source":    msg.Source,
		"heapFree":  msg.HeapFree,
		"heapMin":   msg.HeapMin,
		"metrics":   msg.Metrics,
		"tempC":     msg.TempC,
	}

	// เพิ่ม ip จาก health (ทั้งตอน create และ update)
	if v := strings.TrimSpace(msg.Ip); v != "" {
		set["ip"] = v
	}

	if len(sensors) > 0 {
		set["sensor"] = sensors
	}
	if msg.HealthInterval > 0 {
		set["healthInterval"] = msg.HealthInterval
	}
	if v := strings.TrimSpace(msg.Name); v != "" {
		set["name"] = v
	}

	update := bson.M{
		"$set": set,
		"$setOnInsert": bson.M{
			"hwId":           hwID,
			"approved":       false,
			"alarm":          false,
			"brand":          "kcontrol",
			"dateTimeCreate": now,
			"stats": bson.M{
				"online":            0,
				"warning":           0,
				"offline":           0,
				"alarmCount":        0,
				"sensorDetectCount": 0,
			},

			// initial status แค่ตอนสร้างใหม่
			"status":           "online",
			"message":          firstNonEmpty(msg.Message, "healthy"),
			"topic":            "kcontrol.health",
			"kind":             "health",
			"lastStatusChange": now,

			// default fields/mqtt ที่คุณเซ็ตไว้ก่อนหน้า
			"owner":       "",
			"phone":       "",
			"description": "",
			"lat":         0.0,
			"lng":         0.0,
			"district":    "",
			"mqtt": bson.M{
				"server":       "aliza.k-lynx.com",
				"port":         30883,
				"username":     "klynx",
				"password":     "AH73oaOVnLKi",
				"certificated": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
			},
		},
		"$unset": bson.M{
			"alarmInterval": "",
			"state":         "",
			// "alarm":         "",
			"lastTelemetry": "",
			"interval_IN_1": "",
			"interval_IN_2": "",
			"interval_IN_3": "",
			"interval_IN_4": "",
		},
	}

	opts := options.Update().SetUpsert(true)
	res, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}
	if res.UpsertedCount > 0 {
		log.Debug().Str("hwId", hwID).Msg("🆕 Device upserted (insert from health)")
	} else {
		log.Debug().Str("hwId", hwID).Msg("🆙 Device updated from health")
	}
	return nil
}

/* ===== helpers ===== */

func strOrEmpty(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

// แปลง v (string/number/json.Number) -> int64 > 0
func parsePositiveInt64(x any) (int64, bool) {
	switch t := x.(type) {
	case int64:
		if t > 0 {
			return t, true
		}
	case int:
		n := int64(t)
		if n > 0 {
			return n, true
		}
	case float64:
		n := int64(t)
		if n > 0 {
			return n, true
		}
	case float32:
		n := int64(t)
		if n > 0 {
			return n, true
		}
	case json.Number:
		if n, err := t.Int64(); err == nil && n > 0 {
			return n, true
		}
		if f, err := t.Float64(); err == nil {
			n := int64(f)
			if n > 0 {
				return n, true
			}
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			n := int64(f)
			if n > 0 {
				return n, true
			}
		}
	}
	return 0, false
}

// v = ค่า interval_IN_x จาก MQTT (string หรือ number)
// id = "interval_IN_1" ... "interval_IN_4"
// name = default display name ("SENSOR1" ... "SENSOR4")
func toIntervalSpec(v any, id, name string) *kctrlmod.IntervalSpec {
	if v == nil {
		return nil
	}
	n, ok := parsePositiveInt64(v)
	if !ok {
		return nil
	}
	return &kctrlmod.IntervalSpec{
		ID:     id,
		Name:   name,
		Values: n,
	}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ถ้าไม่มี record → ถือว่ายังไม่ approve
// ถ้า field approved ไม่มี (legacy device เก่า) → ถือว่า approved = true
func isDeviceApproved(ctx context.Context, hwID string) (bool, error) {
	if config.MongoClient == nil || hwID == "" {
		// ไม่มี Mongo → ปล่อยผ่าน (กันระบบล่ม)
		return true, nil
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol")

	var d kctrlmod.DeviceApproval
	err := coll.FindOne(ctx, bson.M{"hwId": hwID}).Decode(&d)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// ยังไม่เคยมี device นี้ → ยังไม่ approve
			return false, nil
		}
		return false, err
	}

	// legacy row ที่ไม่มี field approved → treat as approved
	if d.Approved == nil {
		return true, nil
	}
	return *d.Approved, nil
}
