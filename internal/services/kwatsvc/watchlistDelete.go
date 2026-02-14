// internal/services/kwatsvc/watchlistDelelete.go
package kwatsvc

import (
	"context"
	"errors"
	"time"

	"klynx/config"
	"klynx/internal/kafka"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func WatchlistDelete(ctx context.Context, idOrIdCard string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/kwatsvc",             // tracerName
		"watchlist.WatchlistDelete", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kwatsvc", "WatchlistDelete",
	)
	defer end()
	traceID := traceutil.TraceIDFromCtx(ctx)
	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Debug().Str("idOrIdCard", idOrIdCard).Msg("WatchlistDelete")

	coll := config.DB.Collection(watchlistColl)

	// ------- เลือก filter: ถ้า parse เป็น ObjectID ได้ -> ค้นด้วย _id, ไม่ได้ -> ค้นด้วย idcard -------
	var filter bson.M
	if oid, err := primitive.ObjectIDFromHex(idOrIdCard); err == nil {
		// _id เจาะจงแล้ว ไม่ต้องกรอง isDeleted
		filter = bson.M{"_id": oid}
	} else {
		// กรณี idcard: ให้หาเฉพาะ doc ที่ยัง active
		filter = bson.M{"idcard": idOrIdCard, "isDeleted": false}
	}

	// โหลดข้อมูลที่ handler อาจใช้ (เช่น purge S3) + ต้องมี _id จริงเสมอเพื่อนำไปใส่ event
	var doc struct {
		ID             primitive.ObjectID `bson:"_id"`
		External       bson.M             `bson:"external,omitempty"`
		PhotoKey       string             `bson:"photoKey,omitempty"`
		PhotoOriginKey string             `bson:"photoOriginKey,omitempty"`
		PhotoFaceKey   string             `bson:"photoFaceKey,omitempty"`
		Rev            int64              `bson:"rev,omitempty"`
		State          string             `bson:"state,omitempty"`
	}
	if err := coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// ✅ ถ้าไม่เจอใน DB → ยังต้องยิง Kafka Event
			evt := kwatmod.WatchlistEvent{
				ID:      idOrIdCard, // ใช้ค่าที่ user ส่งมาเลย
				Event:   "watchlist.deleted",
				TraceID: traceID,
				Time:    time.Now().UTC().Format(time.RFC3339Nano),
				Rev:     1,
			}

			headers := map[string]string{
				"event":   evt.Event,
				"source":  "kwatsvc",
				"schema":  "kwatch/WatchlistDelete/1",
				"traceId": traceID,
			}

			const topicWatchlist = "kwatch.watchlist"
			if err := kafka.PublishEventTo(ctx, topicWatchlist, evt.ID, evt, headers); err != nil {
				log.Error().Err(err).Str("docId", evt.ID).Msg("kafka_emit_failed (not found case)")
			} else {
				log.Warn().Str("docId", evt.ID).Msg("📡 published watchlist.deleted (not found in DB)")
			}

			return nil // ✅ ไม่ต้อง return ErrWatchlistNotFound อีก
		}
		return err
	}

	// อัพเดตสถานะทันที → archiving (แจ้งหน้าบ้าน)
	now := time.Now().UTC()
	_, _ = coll.UpdateByID(ctx, doc.ID, bson.M{
		"$set": bson.M{
			"state":     "archiving",
			"updatedAt": now,
		},
	})

	// คำนวณ rev ใหม่ (ไม่มีเดิม = 1)
	nextRev := doc.Rev + 1
	if nextRev <= 0 {
		nextRev = 1
	}

	// ใช้ _id จริงจากเอกสารเสมอสำหรับ Event/Key
	id := doc.ID.Hex()

	// สร้างอีเวนต์ delete
	evt := kwatmod.WatchlistEvent{
		ID:             id, // สำคัญ: ให้เป็น _id (hex) เสมอเพื่อให้ consumer ใช้ได้แน่นอน
		Event:          "watchlist.deleted",
		TraceID:        traceID,
		Time:           time.Now().UTC().Format(time.RFC3339Nano),
		Rev:            nextRev,
		External:       doc.External,
		PhotoKey:       doc.PhotoKey, // ให้ consumer ตัดสินใจ purge S3 หรือ lookup เองก็ได้
		PhotoOriginKey: doc.PhotoOriginKey,
		PhotoFaceKey:   doc.PhotoFaceKey,
	}

	// override เฉพาะ schema; source ใช้ EVENT_SOURCE จาก ENV
	headers := map[string]string{
		"event":   evt.Event,
		"source":  "kwatsvc",
		"schema":  "kwatch/WatchlistDelete/1",
		"traceId": traceID, // keep human-friendly traceId ด้วย
	}
	const topicWatchlist = "kwatch.watchlist"
	if err := kafka.PublishEventTo(ctx, topicWatchlist, id, evt, headers); err != nil {
		log.Error().Err(err).Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_failed")
	} else {
		log.Info().Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_ok")
	}

	log.Info().Str("id", id).Int64("rev", nextRev).Msg("🛰 published watchlist.deleted")
	return nil
}
