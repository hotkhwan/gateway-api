// internal/services/kschsvc/videoCreate.go
package kschsvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	minioSDK "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func VideoCreate(ctx context.Context, req kschmod.VideoCreateRequest) (primitive.ObjectID, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kschsvc", // tracerName
		"search.VideoCreate",                      // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kschsvc", "VideoCreate",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return primitive.NilObjectID, fmt.Errorf("name is required")
	}
	if req.VideoR == nil || req.VideoSize <= 0 {
		return primitive.NilObjectID, fmt.Errorf("%w", ErrVideoRequired)
	}
	if req.VideoSize > maxVideoBytes {
		return primitive.NilObjectID, fmt.Errorf("file too large: max 10GB")
	}

	// ตรวจชนิดไฟล์จากนามสกุล + header (รองรับ mp4/avi/mov/mkv)
	ct, ext, ok := pickVideoContentType(req.VideoFileName, req.VideoContentType)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("unsupported video type (allow: mp4, avi, mov, mkv)")
	}

	newID := primitive.NewObjectID()
	// key := fmt.Sprintf("watchlist/%s_face%s", newID.Hex(), extForMime(origMime))
	key := fmt.Sprintf("vdoSch/%s/%s%s", newID.Hex(), name, ext)

	// อัปโหลดแบบสตรีม (ไม่โหลดทั้งไฟล์เข้า memory)
	startS3 := time.Now()
	if _, err := config.S3Client.PutObject(
		ctx,
		videoBucket,
		key,
		req.VideoR,
		req.VideoSize,
		minioSDK.PutObjectOptions{ContentType: ct},
	); err != nil {
		log.Error().Err(err).Str("key", key).Int64("bytes", req.VideoSize).Msg("💥 upload s3 failed")
		return primitive.NilObjectID, fmt.Errorf("upload s3 failed: %w", err)
	}
	log.Info().
		Str("key", key).
		Int64("bytes", req.VideoSize).
		Str("contentType", ct).
		Dur("took", time.Since(startS3)).
		Msg("✅ uploaded video to S3")

	// insert Mongo
	videoPath := fmt.Sprintf("vdoSch/%s", newID.Hex())
	noSkip := true
	skipFrame := 0
	now := time.Now().UTC()
	statusVal := parseBoolStr(req.Status, true)
	doc := bson.M{
		"_id":              newID,
		"name":             name,
		"description":      strings.TrimSpace(req.Description),
		"status":           statusVal,
		"state":            ifEmptyStr(strings.TrimSpace(req.State), "created"),
		"videoKey":         key,
		"videoContentType": ct,
		"videoSize":        req.VideoSize,
		"videoPath":        videoPath,
		"noSkip":           noSkip,
		"skipFrame":        skipFrame,
		"createdAt":        now,
		"updatedAt":        now,
	}

	startDB := time.Now()
	if _, err := stomongo.InsertOne(ctx, videoColl, doc); err != nil {
		_ = config.S3Client.RemoveObject(ctx, videoBucket, key, minioSDK.RemoveObjectOptions{})
		log.Error().Err(err).Str("docId", newID.Hex()).Dur("took", time.Since(startDB)).Msg("💥 mongo insert failed (rolled back S3)")
		return primitive.NilObjectID, fmt.Errorf("mongo insert failed: %w", err)
	}
	log.Info().Str("docId", newID.Hex()).Dur("took", time.Since(startDB)).Msg("✅ video inserted")

	// Emit Kafka
	// const topic = "analyticsearch.search"
	topic := strings.TrimSpace(os.Getenv("KSCH_KAFKA_TOPIC"))
	if topic == "" {
		topic = "ksearch.video" // ✅ ค่า default
	}

	if strings.EqualFold(os.Getenv("KSCH_KAFKA_DISABLED"), "true") {
		log.Info().Str("docId", newID.Hex()).
			Msg("↷ Kafka disabled by KSCH_KAFKA_DISABLED=true (skip publish)")
	} else {
		evt := kschmod.VideoEvent{
			ID:               newID.Hex(),
			Event:            "video.created",
			Time:             now.Format(time.RFC3339Nano),
			Name:             name,
			Status:           statusVal,
			State:            doc["state"].(string),
			VideoKey:         key,
			VideoContentType: ct,
			VideoSize:        req.VideoSize,
			VideoPath:        videoPath,
			NoSkip:           noSkip,
			SkipFrame:        skipFrame,
		}
		headers := map[string]string{
			"event":  evt.Event,
			"source": "kschsvc",
			"schema": "ksearch/videoCreate/1",
		}

		if err := kafka.PublishEventTo(ctx, topic, newID.Hex(), evt, headers); err != nil {
			// ถ้าไม่มี topic ให้ลดระดับ log เป็น warn และไม่ fail flow
			if strings.Contains(err.Error(), "Unknown Topic Or Partition") {
				log.Warn().Err(err).Str("docId", evt.ID).
					Str("topic", topic).Msg("⚠️ topic not found; event dropped (consider creating the topic)")
			} else {
				log.Error().Err(err).Str("docId", evt.ID).
					Str("topic", topic).Msg("❌ failed to emit event")
			}
		} else {
			log.Info().Str("docId", evt.ID).Str("topic", topic).Msg("📤 emitted event")
		}
	}

	log.Info().Str("docId", newID.Hex()).Msg("🎉 create video done")
	return newID, nil
}

// ---- helpers ----

func pickVideoContentType(filename, headerCT string) (ct, ext string, ok bool) {
	// normalize ext
	ext = strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4":
		return "video/mp4", ".mp4", true
	case ".avi":
		return "video/x-msvideo", ".avi", true
	case ".mov":
		return "video/quicktime", ".mov", true
	case ".mkv":
		return "video/x-matroska", ".mkv", true
	}

	// fallback จาก header (กรณีอัปโหลดไม่มีนามสกุล)
	switch strings.ToLower(strings.TrimSpace(headerCT)) {
	case "video/mp4":
		return "video/mp4", ".mp4", true
	case "video/x-msvideo", "video/avi":
		return "video/x-msvideo", ".avi", true
	case "video/quicktime":
		return "video/quicktime", ".mov", true
	case "video/x-matroska", "application/x-matroska":
		return "video/x-matroska", ".mkv", true
	}
	return "", "", false
}

func parseBoolStr(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return def
	}
}

func ifEmptyStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
