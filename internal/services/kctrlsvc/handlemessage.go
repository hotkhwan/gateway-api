// internal/services/kctrlsvc/handlemessage.go
package kctrlsvc

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kcontrolmsg"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func HandleHealth(ctx context.Context, msg kctrlmod.HealthMessage) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "status.HandleHealth", "kctrlsvc", "HandleHealth")
	defer end()

	// health ทั่วไปไม่ต้องเขียน Mongo แล้ว (ทำไปแล้วใน MQTT handler)
	// ใช้เฉพาะเคสที่อุปกรณ์ส่ง message = "getconfig"
	if strings.TrimSpace(msg.Message) != "getconfig" {
		log.Debug().
			Str("hwId", msg.HwID).
			Str("message", msg.Message).
			Msg("📥 Health message (no-op, state already handled in MQTT handler)")
		return
	}

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol")

	log.Debug().
		Str("hwId", msg.HwID).
		Msg("📥 HandleHealth getconfig")

	// หา device
	var device kctrlmod.Device
	if err := coll.FindOne(cctx, bson.M{"hwId": msg.HwID}).Decode(&device); err != nil {
		if err == mongo.ErrNoDocuments {
			log.Warn().Str("hwId", msg.HwID).Msg("⚠️ Device not found for getconfig")
			return
		}
		log.Error().Err(err).Msg("❌ FindOne in getconfig failed")
		return
	}
	if !device.Approved {
		log.Warn().Str("hwId", msg.HwID).Msg("⛔ Device not approved for config")
		return
	}

	// สร้าง payload config สำหรับตอบกลับอุปกรณ์
	resp := map[string]any{
		"hwId":              device.HwID,
		"setHealthInterval": device.HealthInterval,
	}

	inputs := map[string]*kctrlmod.IntervalSpec{
		"interval_IN_1": device.IntervalIN1,
		"interval_IN_2": device.IntervalIN2,
		"interval_IN_3": device.IntervalIN3,
		"interval_IN_4": device.IntervalIN4,
	}
	for k, v := range inputs {
		if v != nil {
			if _, ok := resp["inputs"]; !ok {
				resp["inputs"] = map[string]*kctrlmod.IntervalSpec{}
			}
			resp["inputs"].(map[string]*kctrlmod.IntervalSpec)[k] = v
		}
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Msg("❌ Marshal config payload failed")
		return
	}
	if err := kcontrolmsg.PublishToMQTT("kcontrol.control", string(payload)); err != nil {
		log.Error().Err(err).Str("topic", "kcontrol.control").Msg("❌ MQTT publish failed")
		return
	}
	log.Debug().Str("payload", string(payload)).Msg("📤 Config sent via MQTT (getconfig)")
}

/* ===== Alarm & Sensor ===== */

func HandleAlarm(ctx context.Context, msg kctrlmod.AlarmMessage) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "status.HandleAlarm", "kctrlsvc", "HandleAlarm")
	defer end()
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	devColl := db.Collection("kcontrol")
	evColl := db.Collection("kcontrol_events")
	almColl := db.Collection("kcontrol_alarms") // collection เก็บสถานะ alarm ล่าสุด
	log.Debug().
		Str("hwId", msg.HwID).
		Msg("✅ MSG update  (HandleAlarm)")
	// หา device
	var device kctrlmod.Device
	if err := devColl.FindOne(cctx, bson.M{"hwId": msg.HwID}).Decode(&device); err != nil {
		if err == mongo.ErrNoDocuments {
			log.Warn().Str("hwId", msg.HwID).Msg("⚠️ Device not found, drop alarm")
			return
		}
		log.Error().Err(err).Str("hwId", msg.HwID).Msg("❌ FindOne device failed in HandleAlarm")
		return
	}
	if !device.Approved {
		log.Warn().Str("hwId", msg.HwID).Msg("⛔ Device not approved, skip alarm")
		return
	}

	dt := msg.Timestamp
	if dt.IsZero() {
		dt = time.Now()
	}

	// fallback message ถ้าไม่ระบุ
	message := msg.Message
	if strings.TrimSpace(message) == "" && msg.Sensor != nil {
		message = "sensor" + itoa(*msg.Sensor) + " alarm"
	}

	now := time.Now()

	// ---------- 1) update device document ใน kcontrol ----------
	devFilter := bson.M{"hwId": msg.HwID}
	devUpdate := bson.M{
		"$set": bson.M{
			"alarm":          true, // มี alarm active
			"lastSeen":       now,
			"dateTimeUpdate": now,
			"alarmInfo": bson.M{ // optional: เก็บรายละเอียด last alarm
				"message":   message,
				"sensor":    msg.Sensor,
				"cycleId":   msg.CycleID,
				"startedAt": msg.StartedAt,
				"alarmAt":   msg.AlarmAt,
				"elapsed":   msg.Elapsed,
				"updatedAt": now,
			},
		},
		"$inc": bson.M{
			"stats.alarmCount": 1, // ✅ นับ alarm
		},
	}

	if res, err := devColl.UpdateOne(cctx, devFilter, devUpdate); err != nil {
		log.Error().
			Err(err).
			Str("hwId", msg.HwID).
			Msg("❌ Failed to update device alarm state")
	} else {
		log.Debug().
			Str("hwId", msg.HwID).
			Int64("matched", res.MatchedCount).
			Int64("modified", res.ModifiedCount).
			Msg("✅ Device alarm state updated (HandleAlarm)")
	}

	// ---------- 2) insert event log ----------
	ev := bson.M{
		"kind":     "alarms",
		"topic":    msg.Topic,
		"hwId":     msg.HwID,
		"datetime": dt,
		"device":   device.ID,
		"name":     device.Name,
		"district": device.District,
		"lat":      device.Lat,
		"lng":      device.Lng,
		"brand":    device.Brand,

		"message": message,
		"sensor":  msg.Sensor,
		"cycleId": msg.CycleID,
		"startedAt": func() *int64 {
			if msg.StartedAt != nil && *msg.StartedAt > 0 {
				return msg.StartedAt
			}
			return nil
		}(),
		"alarmAt": func() *int64 {
			if msg.AlarmAt != nil && *msg.AlarmAt > 0 {
				return msg.AlarmAt
			}
			return nil
		}(),
		"elapsed": msg.Elapsed,
	}

	if _, err := evColl.InsertOne(cctx, ev); err != nil {
		log.Error().Err(err).Msg("❌ Failed to insert alarm event")
		return
	}
	log.Debug().Str("hwId", msg.HwID).Msg("✅ Alarm event inserted to kcontrol_events")

	// ---------- 3) upsert kcontrol_alarms (สถานะ alarm ล่าสุด) ----------
	cycleId := msg.CycleID
	if strings.TrimSpace(cycleId) == "" {
		cycleId = "default"
	}

	filter := bson.M{
		"hwId":    msg.HwID,
		"sensor":  msg.Sensor,
		"cycleId": cycleId,
	}

	alarmDoc := bson.M{
		"hwId":     msg.HwID,
		"device":   device.ID,
		"name":     device.Name,
		"district": device.District,
		"lat":      device.Lat,
		"lng":      device.Lng,
		"brand":    device.Brand,

		"message":  message,
		"sensor":   msg.Sensor,
		"cycleId":  cycleId,
		"datetime": dt,
		"startedAt": func() *int64 {
			if msg.StartedAt != nil && *msg.StartedAt > 0 {
				return msg.StartedAt
			}
			return nil
		}(),
		"alarmAt": func() *int64 {
			if msg.AlarmAt != nil && *msg.AlarmAt > 0 {
				return msg.AlarmAt
			}
			return nil
		}(),
		"elapsed":   msg.Elapsed,
		"status":    "active",
		"updatedAt": now,
	}

	up := bson.M{
		"$set": alarmDoc,
		"$setOnInsert": bson.M{
			"createdAt": now,
		},
	}

	// ✅ ใช้ FindOneAndUpdate เพื่อให้ได้ _id กลับมาแน่นอนทุกครั้ง
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var updated bson.M
	if err := almColl.FindOneAndUpdate(cctx, filter, up, opts).Decode(&updated); err != nil {
		log.Error().Err(err).Msg("❌ Failed to upsert kcontrol_alarms (FindOneAndUpdate)")
		return
	}

	id, ok := updated["_id"].(primitive.ObjectID)
	if !ok {
		// กันพลาด: _id ไม่ใช่ ObjectID (unlikely) แต่ถ้าเจอจะได้ไม่ publish แบบมั่ว
		log.Error().
			Interface("_id", updated["_id"]).
			Msg("❌ kcontrol_alarms _id is not ObjectID; skip mqtt publish")
		return
	}

	log.Debug().
		Str("hwId", msg.HwID).
		Str("id", id.Hex()).
		Msg("✅ Alarm upserted to kcontrol_alarms")

	// ---------- 4) publish public MQTT alarm result ----------
	out := map[string]any{
		"id":        id.Hex(),
		"hwId":      msg.HwID,
		"sensor":    msg.Sensor,
		"cycleId":   cycleId,
		"status":    "active",
		"message":   message,
		"datetime":  dt, // time.Time -> marshal เป็น RFC3339
		"updatedAt": now,
	}

	// อยากแนบ metadata device ก็ได้ (เลือกเท่าที่จำเป็น)
	out["device"] = map[string]any{
		"id":       device.ID,
		"name":     device.Name,
		"district": device.District,
		"lat":      device.Lat,
		"lng":      device.Lng,
		"brand":    device.Brand,
	}

	payload, err := json.Marshal(out)
	if err != nil {
		log.Error().Err(err).Msg("❌ Marshal alarmResult payload failed")
		return
	}

	if err := kcontrolmsg.PublishToMQTT("kcontrol.alarmResult", string(payload)); err != nil {
		log.Error().Err(err).Str("topic", "kcontrol.alarmResult").Msg("❌ MQTT publish alarmResult failed")
		return
	}

	log.Debug().
		Str("topic", "kcontrol.alarmResult").
		Str("hwId", msg.HwID).
		Str("id", id.Hex()).
		Msg("📤 Alarm result published to MQTT")
}

func HandleSensor(ctx context.Context, msg kctrlmod.SensorMessage) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "status.HandleSensor", "kctrlsvc", "HandleSensor")
	defer end()
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	devColl := db.Collection("kcontrol")
	evColl := db.Collection("kcontrol_events")

	// หา device
	var device kctrlmod.Device
	if err := devColl.FindOne(cctx, bson.M{"hwId": msg.HwID}).Decode(&device); err != nil {
		log.Warn().Str("hwId", msg.HwID).Msg("⚠️ Device not found, drop sensor event")
		return
	}
	if !device.Approved {
		log.Warn().Str("hwId", msg.HwID).Msg("⛔ Device not approved, skip sensor")
		return
	}

	now := time.Now()

	// ---------- อัปเดตสถิติ sensorDetectCount ----------
	devFilter := bson.M{"hwId": msg.HwID}
	devUpdate := bson.M{
		"$set": bson.M{
			"lastSeen":       now,
			"dateTimeUpdate": now,
		},
		"$inc": bson.M{
			"stats.sensorDetectCount": 1, // ✅ นับ detect sensor
		},
	}

	if res, err := devColl.UpdateOne(cctx, devFilter, devUpdate); err != nil {
		log.Error().
			Err(err).
			Str("hwId", msg.HwID).
			Msg("❌ Failed to update device sensor stats")
	} else {
		log.Debug().
			Str("hwId", msg.HwID).
			Int64("matched", res.MatchedCount).
			Int64("modified", res.ModifiedCount).
			Msg("✅ Device sensor stats updated (HandleSensor)")
	}

	// ---------- บันทึก event แบบแบน ----------
	started := msg.StartedAt
	ev := bson.M{
		"kind":     "sensor",
		"topic":    msg.Topic,
		"hwId":     msg.HwID,
		"datetime": msg.Timestamp,
		"device":   device.ID,
		"name":     device.Name,
		"district": device.District,
		"lat":      device.Lat,
		"lng":      device.Lng,
		"brand":    device.Brand,

		"message":  msg.Event,
		"sensor":   &msg.Sensor,
		"event":    msg.Event,
		"duration": &msg.Duration,
		"cycleId":  msg.CycleID,
		"startedAt": func() *int64 {
			if started > 0 {
				return &started
			}
			return nil
		}(),
	}

	if _, err := evColl.InsertOne(cctx, ev); err != nil {
		log.Error().Err(err).Msg("❌ Failed to insert sensor event")
		return
	}
	log.Debug().
		Str("hwId", msg.HwID).
		Int("sensor", msg.Sensor).
		Msg("✅ Sensor event inserted")
}

func HandleEvent(ctx context.Context, msg kctrlmod.EventMAPMessage) {
	log := logger.FromCtx(ctx, "kctrlsvc", "HandleEvent")

	gw := getGateways()
	if gw.MQTTBridge == nil {
		log.Warn().Msg("MQTT bridge not configured; skip relay to MQTT")
		return
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("marshal event failed")
		return
	}

	if err := gw.MQTTBridge.HandleHealthEvent(ctx, raw); err != nil {
		log.Error().Err(err).Msg("HandleHealthEvent failed")
		return
	}

	// ไม่อ้าง field ที่ struct ไม่มีแล้ว
	log.Debug().Msg("✅ Event forwarded to MQTT")
}

// ===== utils =====

func itoa(i int) string { return strconv.Itoa(i) }
