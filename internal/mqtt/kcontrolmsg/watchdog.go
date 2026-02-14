// internal/adapters/mqtt/kcontrol/watchdog.go
package kcontrolmsg

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"klynx/config"
	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/models/kctrlmod"
)

func CheckDeviceStatus() {
	log := logger.WithMeta("kcontrol", "internal-mqtt-watchdog-CheckDeviceStatus")
	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now()
	cursor, err := coll.Find(ctx, bson.M{"approved": true}, options.Find().SetBatchSize(200))
	if err != nil {
		log.Error().Err(err).Str("collection", "kcontrol").Msg("Failed to fetch devices")
		return
	}
	defer cursor.Close(ctx)

	log.Debug().Msg("Running CheckDeviceStatus")

	for cursor.Next(ctx) {
		var device kctrlmod.Device
		if err := cursor.Decode(&device); err != nil {
			log.Warn().Err(err).Msg("Decode error")
			continue
		}

		warnMs, offMs := resolveThresholdMs(device)

		diff := now.Sub(device.LastSeen)
		newStatus := "online"
		switch {
		case diff > time.Duration(offMs)*time.Millisecond:
			newStatus = "offline"
		case diff > time.Duration(warnMs)*time.Millisecond:
			newStatus = "warning"
		}

		if newStatus != device.Status {
			update := bson.M{
				"$set": bson.M{
					"status":           newStatus,
					"dateTimeUpdate":   now,
					"lastStatusChange": now,
					"inactiveForMs":    diff.Milliseconds(),

					// เพิ่ม field ตาม requirement
					"topic":           "kcontrol.health",
					"kind":            "health",
					"message":         newStatus,
					"source":          "watchdog",
					"timestamp":       now.Format("2006-01-02T15:04:05"), // หรือ RFC3339 ก็ได้
					"warnThresholdMs": warnMs,
					"offThresholdMs":  offMs,
				},
				"$inc": bson.M{},
			}
			switch newStatus {
			case "warning":
				update["$inc"].(bson.M)["stats.warning"] = 1
			case "offline":
				update["$inc"].(bson.M)["stats.offline"] = 1
			case "online":
				update["$inc"].(bson.M)["stats.online"] = 1
			}

			if _, err := coll.UpdateOne(ctx, bson.M{"_id": device.ID, "status": device.Status}, update); err != nil {
				log.Error().Err(err).Str("deviceId", device.ID.Hex()).Str("status", newStatus).Msg("Failed to update status")
				continue
			}

			log.Debug(). //. Status updated
					Str("deviceId", device.ID.Hex()).
					Str("from", device.Status).
					Str("to", newStatus).
					Dur("inactiveFor", diff).
					Msg("✅ Status updated")

			// Emit Kafka (ตามโค้ดเดิม, ผมแค่ขอแนบ stats/sensor ไปด้วย)
			sendWatchdogHealth(ctx, device, newStatus, diff,
				time.Duration(warnMs)*time.Millisecond,
				time.Duration(offMs)*time.Millisecond,
				now)
		} else {
			log.Debug().
				Str("deviceId", device.ID.Hex()).
				Dur("diff", diff).
				Str("current", device.Status).
				Str("calculated", newStatus).
				Msg("🧮 Status evaluation")
		}
	}
}

// resolveThresholdMs:
//
// priority:
//  1. ถ้า device.WarnThresholdMs / OffThresholdMs > 0 → ใช้ตามนั้น
//  2. ถ้า healthInterval > 0 → warn = HI * WARN_MULT, off = HI * OFF_MULT
//  3. ถ้ายังไม่มี → fallback จาก KC_WARN_SECONDS / KC_OFF_SECONDS
func resolveThresholdMs(d kctrlmod.Device) (warnMs int64, offMs int64) {
	// เริ่มจากค่าที่เก็บใน DB ก่อน (รองรับ override manual)
	warnMs = d.WarnThresholdMs
	offMs = d.OffThresholdMs

	hi := d.HealthInterval
	warnMul := envFloat("KCTRL_WARN_MULTIPLIER", 3.0)   // hi * 3
	offMul := envFloat("KCTRL_OFFLINE_MULTIPLIER", 5.0) // hi * 5

	if warnMs <= 0 && hi > 0 {
		warnMs = int64(float64(hi) * warnMul)
	}
	if offMs <= 0 && hi > 0 {
		offMs = int64(float64(hi) * offMul)
	}

	// fallback เป็น seconds ถ้า healthInterval ยังไม่ถูกตั้ง
	if warnMs <= 0 {
		defWarnSec := envInt("KC_WARN_SECONDS", 30)
		warnMs = int64(defWarnSec * 1000)
	}
	if offMs <= 0 {
		defOffSec := envInt("KC_OFF_SECONDS", 60)
		offMs = int64(defOffSec * 1000)
	}

	// กันความผิดพลาด: off ต้องมากกว่า warn
	if offMs <= warnMs {
		offMs = warnMs + 10000 // +10s safety
	}
	return
}

func envInt(key string, def int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	}
	return def
}

func sendWatchdogHealth(ctx context.Context, device kctrlmod.Device, newStatus string, inactiveFor time.Duration, warnTh, offTh time.Duration, now time.Time) {
	log := logger.WithMeta("kcontrol", "watchdog.emit")
	key := device.HwID
	if key == "" {
		key = device.ID.Hex()
	}

	payload := map[string]any{
		"topic":           "kcontrol.events",
		"kind":            "health",
		"message":         newStatus,
		"status":          newStatus,
		"timestamp":       now.UTC().Format(time.RFC3339),
		"hwId":            device.HwID,
		"ip":              device.Ip,
		"tempC":           device.TempC,
		"deviceId":        device.ID.Hex(),
		"name":            device.Name,
		"lastSeen":        device.LastSeen.UTC().Format(time.RFC3339),
		"healthInterval":  int64(device.HealthInterval),
		"inactiveForMs":   inactiveFor.Milliseconds(),
		"warnThresholdMs": warnTh.Milliseconds(),
		"offThresholdMs":  offTh.Milliseconds(),
		"source":          "watchdog",
		// แนบสถิติ/เซ็นเซอร์ไปด้วย ให้ consumer เอาไปเขียน DB ง่ายขึ้น
		"stats":  device.Stats,
		"sensor": device.Sensor,
	}

	headers := map[string]string{
		"schema": "kcontrol/watchdog-status/1",
		"event":  "kcontrol.watchdog.status_changed",
		"kind":   "health",
		"source": "klynx",
		"rev":    "0",
	}

	if err := kafka.PublishEventTo(ctx, "kcontrol.events", key, payload, headers); err != nil {
		log.Error().Err(err).Str("deviceId", device.ID.Hex()).Msg("❌ Kafka publish failed (watchdog)")
		return
	}
	log.Debug(). // Watch log on line offlice
			Str("deviceId", device.ID.Hex()).
			Str("hwId", device.HwID).
			Str("to", newStatus).
			Msg("📤 Watchdog health emitted")
}
