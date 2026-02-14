// internal/services/kschsvc/videoDelete.go
package kschsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"klynx/config"
	"klynx/internal/repo/stomongo"
	"klynx/utils/traceutil"

	minioSDK "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func VideoDelete(ctx context.Context, id primitive.ObjectID) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/kschsvc",      // tracerName
		"search.VideoDelete", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kschsvc", "VideoDelete",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	traceID := traceIDFromCtx(ctx)
	startAll := time.Now()

	if id.IsZero() {
		return fmt.Errorf("%w", ErrInvalidObjectID)
	}

	// 1) ดึงเอกสารเพื่อรู้ videoKey ก่อน
	var doc struct {
		Name     string `bson:"name"`
		VideoKey string `bson:"videoKey"`
	}
	if err := stomongo.FindOne(ctx, videoColl, bson.M{"_id": id}, &doc); err != nil {
		// แยก not found ตาม error ที่ไลบรารีคืนมา
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w", ErrVideoNotFound)
		}
		return err
	}

	// 2) ลบ S3 (best-effort)
	if doc.VideoKey != "" {
		if err := config.S3Client.RemoveObject(ctx, videoBucket, doc.VideoKey, minioSDK.RemoveObjectOptions{}); err != nil {
			// log เตือน แต่ไม่ fail ทั้งงาน
			log.Warn().
				Err(err).
				Str("traceID", traceID).
				Str("bucket", videoBucket).
				Str("key", doc.VideoKey).
				Msg("⚠️ failed to remove video object from S3 (ignored)")
		}
	}

	// 3) ลบ Mongo
	// 2) Soft delete: อัปเดตสถานะ แทนการลบ
	now := time.Now().UTC()
	filter := bson.M{
		"_id": id,
		// กันลบซ้ำ: อนุญาตอัปเดตเฉพาะรายการที่ยังไม่ถูกลบ
		"$or": []bson.M{
			{"deletedAt": bson.M{"$exists": false}},
			{"state": bson.M{"$ne": "deleted"}},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"state":     "deleted",
			"deletedAt": now,
			"updatedAt": now,
			// จะมี "deletedBy" ก็ใส่เพิ่มได้ ถ้ามี user ใน ctx
		},
	}
	_, err := stomongo.UpdateOne(ctx, videoColl, filter, update)
	if err != nil {
		return err
	}
	// if _, err := stomongo.DeleteOne(ctx, videoColl, bson.M{"_id": id}); err != nil {
	// 	return err
	// }

	// // 4) Emit Kafka event: video.deleted
	// evt := struct {
	// 	ID     string `json:"id"`
	// 	Event  string `json:"event"`
	// 	Time   string `json:"time"`
	// 	Name   string `json:"name"`
	// 	Key    string `json:"videoKey"`
	// 	Status string `json:"status"`
	// }{
	// 	ID:     id.Hex(),
	// 	Event:  "video.deleted",
	// 	Time:   time.Now().UTC().Format(time.RFC3339Nano),
	// 	Name:   doc.Name,
	// 	Key:    doc.VideoKey,
	// 	Status: "deleted",
	// }

	// headers := map[string]string{
	// 	"event":  evt.Event,
	// 	"source": "kschsvc",
	// 	"schema": "ksearch/videoDelete/1",
	// }
	// if err := kafka.PublishEventTo(ctx, "ksearch.video", id.Hex(), evt, headers); err != nil {
	// 	log.Warn().
	// 		Err(err).
	// 		Str("traceID", traceID).
	// 		Str("docId", id.Hex()).
	// 		Str("topic", "ksearch.video").
	// 		Msg("⚠️ failed to emit kafka event (ignored)")
	// }

	log.Info().
		Str("traceID", traceID).
		Str("docId", id.Hex()).
		Dur("took_total", time.Since(startAll)).
		Msg("🗑️ video deleted")
	return nil
}
