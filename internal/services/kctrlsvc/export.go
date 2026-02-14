// internal/services/kctrlsvc/export.go
package kctrlsvc

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"klynx/config"
	"klynx/utils/traceutil"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func ExportAlarms(ctx context.Context, c *fiber.Ctx) error {
	ctx, endSpan, log := traceutil.StartLite(
		ctx,
		"klynx/kctrlsvc",
		"alarms.ExportAlarms",
		"kctrlsvc", "ExportAlarms",
	)
	defer endSpan()

	// ป้องกัน query หนัก ๆ แล้ว timeout สั้นไป
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	deviceID := strings.TrimSpace(c.Query("deviceId"))
	dateRange := strings.TrimSpace(c.Query("dateTime"))

	log.Debug().
		Str("deviceId", deviceID).
		Str("dateRange", dateRange).
		Msg("📤 ExportAlarms triggered")

	// ช่วงเวลา default = ล่าสุด 24 ชม.
	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now

	// รองรับรูปแบบ "start,end" เป็น RFC3339/RFC3339Nano (รองรับมี space ด้วย)
	if dateRange != "" {
		parts := strings.Split(dateRange, ",")
		if len(parts) == 2 {
			parse := func(s string) (time.Time, error) {
				s = strings.TrimSpace(s)
				// ลอง RFC3339Nano ก่อน แล้วค่อย fallback RFC3339
				if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
					return t, nil
				}
				return time.Parse(time.RFC3339, s)
			}
			if s, err1 := parse(parts[0]); err1 == nil {
				start = s
			} else {
				log.Warn().Err(err1).Str("value", parts[0]).Msg("⚠️ Failed to parse start time")
			}
			if e, err2 := parse(parts[1]); err2 == nil {
				end = e
			} else {
				log.Warn().Err(err2).Str("value", parts[1]).Msg("⚠️ Failed to parse end time")
			}
			if !end.After(start) {
				// ถ้า end <= start ให้แก้เป็น start+24h ป้องกัน query คว่ำ
				end = start.Add(24 * time.Hour)
				log.Warn().Msg("⚠️ end <= start; adjusted end to start+24h")
			}
		} else {
			log.Warn().Str("dateRange", dateRange).Msg("⚠️ dateRange should be 'start,end'; using default last 24h")
		}
	}

	// Build match stage
	match := bson.M{
		"datetime": bson.M{
			"$gte": start,
			"$lte": end,
		},
	}

	// optional filter by deviceID (ObjectID hex)
	if deviceID != "" {
		if objID, err := primitive.ObjectIDFromHex(deviceID); err == nil {
			match["device"] = objID
		} else {
			log.Warn().Err(err).Str("deviceId", deviceID).Msg("⚠️ Invalid deviceId, skipping filter")
		}
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol_alarms")

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "datetime", Value: -1}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "kcontrol",
			"localField":   "device",
			"foreignField": "_id",
			"as":           "deviceInfo",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$deviceInfo", "preserveNullAndEmptyArrays": true}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Interface("pipeline", pipeline).Msg("❌ Mongo aggregate query failed")
		return c.Status(fiber.StatusInternalServerError).SendString("❌ Failed to query alarms")
	}
	defer cursor.Close(ctx)

	var buf bytes.Buffer

	// ใส่ UTF‑8 BOM ให้ Excel (Windows) แสดงภาษาไทยถูก
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)

	// header
	if err := writer.Write([]string{"Datetime", "Message", "Device Name", "District", "Location"}); err != nil {
		log.Error().Err(err).Msg("❌ Failed to write CSV header")
		return c.Status(fiber.StatusInternalServerError).SendString("❌ Failed to write CSV")
	}

	count := 0
	for cursor.Next(ctx) {
		var r bson.M
		if err := cursor.Decode(&r); err != nil {
			log.Warn().Err(err).Msg("⚠️ Failed to decode alarm record; skipping")
			continue
		}

		// datetime แบบปลอดภัย (กัน panic)
		var dtStr string
		switch v := r["datetime"].(type) {
		case primitive.DateTime:
			dtStr = v.Time().Format("2006-01-02 15:04:05")
		case time.Time:
			dtStr = v.Format("2006-01-02 15:04:05")
		case string:
			dtStr = v
		default:
			dtStr = "-"
		}

		msg := fmt.Sprintf("%v", r["message"])

		// deviceInfo อาจไม่มี (preserveNullAndEmptyArrays=true)
		name, district, location := "-", "-", "-"
		if info, ok := r["deviceInfo"].(bson.M); ok && info != nil {
			if v, ok := info["name"]; ok {
				name = fmt.Sprintf("%v", v)
			}
			if v, ok := info["district"]; ok {
				district = fmt.Sprintf("%v", v)
			}
			// รองรับคีย์ lat/long หรือ lat/lng
			var lat, lng any
			if v, ok := info["lat"]; ok {
				lat = v
			}
			if v, ok := info["long"]; ok {
				lng = v
			} else if v, ok := info["lng"]; ok {
				lng = v
			}
			if lat != nil && lng != nil {
				location = fmt.Sprintf("%v,%v", lat, lng)
			}
		}

		if err := writer.Write([]string{dtStr, msg, name, district, location}); err != nil {
			log.Warn().Err(err).Msg("⚠️ Failed to write CSV row; skipping")
			continue
		}
		count++
	}
	// ตรวจ error จาก cursor
	if err := cursor.Err(); err != nil {
		log.Error().Err(err).Msg("❌ Cursor error during iteration")
		return c.Status(fiber.StatusInternalServerError).SendString("❌ Failed while reading alarms")
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Error().Err(err).Msg("❌ CSV writer error")
		return c.Status(fiber.StatusInternalServerError).SendString("❌ Failed to finalize CSV")
	}

	log.Debug().
		Int("count", count).
		Str("start", start.Format(time.RFC3339)).
		Str("end", end.Format(time.RFC3339)).
		Msg("✅ ExportAlarms complete")

	filename := "kcontrol_alarms_" + time.Now().Format("20060102_150405") + ".csv"
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Send(buf.Bytes())
}
