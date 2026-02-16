// internal/services/kwatsvc/createWatchlist.go
package kwatsvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/hotkhwan/gateway-api/internal/gateways/iboc/watchlist/ibface"

	minioSDK "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// local helper สำหรับนามสกุลไฟล์จาก MIME
func extForMime(mime string) string {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	default:
		return ".jpg"
	}
}

// แปลง string → bool พร้อม default
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

func WatchlistCreate(ctx context.Context, req kwatmod.WatchlistCreateRequest) (primitive.ObjectID, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kwatsvc",             // tracerName
		"watchlist.WatchlistCreate", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kwatsvc", "WatchlistCreate",
	)
	defer end()
	traceID := traceutil.TraceIDFromCtx(ctx)
	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Info().
		Str("traceID", traceID).
		Str("idcard", strings.TrimSpace(req.IdCard)).
		Str("passport", strings.TrimSpace(req.Passport)).
		Msg("svc_start")

	// 1) duplicate check (idcard) — allow if soft-deleted
	if idc := strings.TrimSpace(req.IdCard); idc != "" {
		log.Debug().Str("traceID", traceID).Str("idcard", idc).Msg("🔎 checking duplicate")

		filter := bson.M{
			"idcard": idc,
			"$and": []bson.M{
				{"$or": []bson.M{
					{"deletedAt": bson.M{"$exists": false}},
					{"deletedAt": nil},
				}},
				{"state": bson.M{"$in": []string{"queued", "active", "updated"}}},
			},
		}
		var exists struct {
			ID primitive.ObjectID `bson:"_id"`
		}

		if err := stomongo.FindOne(ctx, watchlistColl, filter, &exists); err == nil && !exists.ID.IsZero() {
			log.Warn().
				Str("traceID", traceID).
				Str("idcard", idc).
				Str("existingDocId", exists.ID.Hex()).
				Str("reason", "duplicateIdcard").
				Msg("🚫 duplicate watchlist")
			return primitive.NilObjectID, fmt.Errorf("%w", ErrDuplicateIDCard)
		}
	}

	// 2) ต้องมีรูป
	if len(req.PhotoFile) == 0 {
		log.Warn().Str("traceID", traceID).Str("reason", "photo_required").Msg("🚫 missing photo file")
		return primitive.NilObjectID, fmt.Errorf("%w", ErrPhotoRequired)
	}

	// 2.1) เตรียมภาพเพื่อทดสอบหน้า
	jpegBytes, _, err := ensureJPEG(req.PhotoFile)
	if err != nil {
		log.Warn().
			Err(err).
			Str("traceID", traceID).
			Str("reason", "bad_photo_format").
			Msg("🚫 photo convert/validate failed")
		return primitive.NilObjectID, fmt.Errorf("%w: %v", ErrBadPhoto, err)
	}

	// 3) วิเคราะห์หน้าด้วย IBOC (ไม่เก็บผลครอป)
	iboc := ibface.NewFromEnv("IBOC_6")
	if !iboc.Configured() {
		log.Error().Str("traceID", traceID).Msg("💥 IBOC not configured for crop")
		return primitive.NilObjectID, fmt.Errorf("iboc not configured for crop")
	}
	startCrop := time.Now()
	if _, _, err := iboc.AnalyzeCrop(ctx, jpegBytes); err != nil {
		if errors.Is(err, ibface.ErrNoFaceDetected) {
			log.Warn().Str("traceID", traceID).Dur("took", time.Since(startCrop)).Str("reason", "no_face_detected").Msg("🚫 face validation failed")
			return primitive.NilObjectID, fmt.Errorf("%w", ErrNoFace)
		}
		log.Warn().Err(err).Str("traceID", traceID).Dur("took", time.Since(startCrop)).Str("reason", "bad_photo").Msg("🚫 face validation error")
		return primitive.NilObjectID, fmt.Errorf("%w: %v", ErrBadPhoto, err)
	}
	log.Info().Str("traceID", traceID).Dur("took", time.Since(startCrop)).Msg("✅ face validated")

	// 4) อัปโหลด S3 ด้วย "ภาพต้นฉบับ"
	origMime := httpDetectContentType(req.PhotoFile)
	if origMime == "" || origMime == "application/octet-stream" {
		origMime = "image/jpeg"
	}
	newID := primitive.NewObjectID()

	// แยก key ชัดเจน
	originKey := fmt.Sprintf("watchlist/%s/origin%s", newID.Hex(), extForMime(origMime))
	faceKey := fmt.Sprintf("watchlist/%s/face.jpg", newID.Hex())
	startS3 := time.Now()
	if _, err := config.S3Client.PutObject(
		ctx,
		"kwatch",
		originKey,
		bytes.NewReader(req.PhotoFile),
		int64(len(req.PhotoFile)),
		minioSDK.PutObjectOptions{ContentType: origMime},
	); err != nil {
		log.Error().Err(err).Str("traceID", traceID).Str("key", originKey).Int("bytes", len(req.PhotoFile)).Msg("💥 upload s3 failed")
		return primitive.NilObjectID, fmt.Errorf("upload s3 failed: %w", err)
	}
	log.Info().
		Str("traceID", traceID).
		Str("originKey", originKey).
		Int("bytes", len(req.PhotoFile)).
		Str("contentType", origMime).
		Dur("took", time.Since(startS3)).
		Msg("✅ uploaded ORIGINAL photo to S3")
	// 5) insert Mongo
	now := time.Now().UTC()
	statusVal := parseBoolStr(req.Status, true)
	doc := bson.M{
		"_id":     newID,
		"traceID": traceID,
		// "type":             req.Type,
		// "personalType":     req.PersonalType,
		// "crimesType":       req.CrimesType,
		"personKey":    req.IdCard,
		"idcard":       req.IdCard,
		"passport":     req.Passport,
		"titlename":    req.TitleName,
		"subTitlename": req.SubTitleName,
		"firstname":    req.FirstName,
		"lastname":     req.LastName,
		"nickname":     req.NickName,
		"sex":          req.Sex,
		"birthday":     req.Birthday,
		// "age":           req.Age,
		"fatherName":    req.FatherName,
		"fatherIdcard":  req.FatherIdCard,
		"motherName":    req.MotherName,
		"motherIdcard":  req.MotherIdCard,
		"maritalStatus": req.MaritalStatus,
		"dateOfDeath":   req.DateOfDeath,
		// "deathStatus":      req.DeathStatus,
		// "policeRegion":     req.PoliceRegion,
		// "policeProvincial": req.PoliceProvincial,
		// "policeStation":    req.PoliceStation,
		"userRecorder": req.UserRecorder,
		"userPosition": req.UserPosition,
		"status":       statusVal,
		"source":       ifEmptyStr(req.Source, "internal"),
		"state":        ifEmptyStr(req.State, "queued"),
		// เปลี่ยนจาก photoKey เดิม → เก็บ origin + face แยก
		"photoOriginKey":         originKey,
		"photoOriginContentType": origMime,
		"photoFaceKey":           "", // ยังไม่สร้าง ให้ handler เขียน
		"photoFaceContentType":   "",
		// เพื่อ compatibility: ตั้ง photoKey = origin ก่อน (ให้ client ยังแสดงรูปได้)
		"photoKey":         originKey,
		"photoContentType": origMime,
		"createdAt":        now,
		"updatedAt":        now,
		"alertType":        req.AlertType,
		"alertDesc":        req.AlertDesc,
		"isDeleted":        false,
	}
	t, tok := atoiOpt(req.Type)
	if !tok || (t != 1 && t != 2) {
		return primitive.NilObjectID, fmt.Errorf("invalid type: must be 1 or 2")
	}
	doc["type"] = t

	if t == 1 {
		v, ok := atoiOpt(req.PersonalType)
		if !ok {
			return primitive.NilObjectID, fmt.Errorf("personalType is required and must be a number when type=1")
		}
		doc["personalType"] = v
	} else { // t == 2
		v, ok := atoiOpt(req.CrimesType)
		if !ok {
			return primitive.NilObjectID, fmt.Errorf("crimesType is required and must be a number when type=2")
		}
		doc["crimesType"] = v
	}
	// if v, ok := atoiOpt(req.Type); ok {
	// 	doc["type"] = v
	// }
	// if v, ok := atoiOpt(req.PersonalType); ok {
	// 	doc["personalType"] = v
	// }
	// if v, ok := atoiOpt(req.CrimesType); ok {
	// 	doc["crimesType"] = v
	// }
	if v, ok := atoiOpt(req.DeathStatus); ok {
		doc["deathStatus"] = v
	}
	if v, ok := atoiOpt(req.PoliceRegion); ok {
		doc["policeRegion"] = v
	}
	if v, ok := atoiOpt(req.PoliceProvincial); ok {
		doc["policeProvincial"] = v
	}
	if v, ok := atoiOpt(req.PoliceStation); ok {
		doc["policeStation"] = v
	}
	if v, ok := atoiOpt(req.Age); ok {
		doc["age"] = v
	}
	startDB := time.Now()
	if _, err := stomongo.InsertOne(ctx, watchlistColl, doc); err != nil {
		_ = config.S3Client.RemoveObject(ctx, "kwatch", originKey, minioSDK.RemoveObjectOptions{})
		log.Error().Err(err).Str("traceID", traceID).Str("docId", newID.Hex()).Dur("took", time.Since(startDB)).Msg("💥 mongo insert failed (rolled back S3)")
		return primitive.NilObjectID, fmt.Errorf("mongo insert failed: %w", err)
	}
	log.Info().Str("traceID", traceID).Str("docId", newID.Hex()).Dur("took", time.Since(startDB)).Msg("✅ watchlist inserted")

	// 6) Emit Kafka event
	const topicWatchlist = "kwatch.watchlist"
	evt := kwatmod.WatchlistEvent{
		ID:                     newID.Hex(),
		Event:                  "watchlist.created",
		TraceID:                traceID,
		Time:                   now.UTC().Format(time.RFC3339Nano),
		Rev:                    0,
		State:                  str(doc["state"]),
		PhotoKey:               str(doc["photoKey"]),
		PhotoContentType:       str(doc["photoContentType"]),
		PhotoOriginKey:         str(doc["photoOriginKey"]),
		PhotoOriginContentType: str(doc["photoOriginContentType"]),
		PhotoFaceKey:           faceKey,
		Type:                   intOf(doc["type"]),
		PersonalType:           intOf(doc["personalType"]),
		CrimesType:             intOf(doc["crimesType"]),
		IdCard:                 str(doc["idcard"]),
		PersonKey:              str(doc["idcard"]),
		Passport:               str(doc["passport"]),
		TitleName:              str(doc["titlename"]),
		SubTitleName:           str(doc["subTitlename"]),
		FirstName:              str(doc["firstname"]),
		LastName:               str(doc["lastname"]),
		NickName:               str(doc["nickname"]),
		Sex:                    str(doc["sex"]),
		Birthday:               str(doc["birthday"]),
		Age:                    intOf(doc["age"]),
		FatherName:             str(doc["fatherName"]),
		FatherIdCard:           str(doc["fatherIdcard"]),
		MotherName:             str(doc["motherName"]),
		MotherIdCard:           str(doc["motherIdcard"]),
		MaritalStatus:          str(doc["maritalStatus"]),
		DeathStatus:            intOf(doc["deathStatus"]),
		DateOfDeath:            str(doc["dateOfDeath"]),
		PoliceRegion:           intOf(doc["policeRegion"]),
		PoliceProvincial:       intOf(doc["policeProvincial"]),
		PoliceStation:          intOf(doc["policeStation"]),
		UserRecorder:           str(doc["userRecorder"]),
		UserPosition:           str(doc["userPosition"]),
		AlertType:              str(doc["alertType"]),
		AlertDesc:              str(doc["alertDesc"]),
	}
	headers := map[string]string{
		"event":          evt.Event,
		"source":         "kwatsvc",
		"schema":         "kwatch/watchlistCreate/1",
		"traceId":        traceID, // keep human-friendly traceId ด้วย
		"imageOriginKey": originKey,
	}
	if err := kafka.PublishEventTo(ctx, topicWatchlist, originKey, evt, headers); err != nil {
		log.Error().Err(err).Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_failed")
	} else {
		log.Info().Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_ok")
	}

	log.Info().Str("traceID", traceID).Str("docId", newID.Hex()).Msg("🎉 create watchlist done")
	return newID, nil
}

// helpers
// ทำให้เป็น JPEG เสมอก่อนส่งไป IBOC (รองรับ PNG/JPEG เบื้องต้น)
func ensureJPEG(src []byte) ([]byte, string, error) {
	// quick sniff
	ct := httpDetectContentType(src)
	switch {
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		// already jpeg
		return src, "image/jpeg", nil
	case strings.Contains(ct, "png"):
		// decode png → encode jpeg
		img, err := pngDecode(bytes.NewReader(src))
		if err != nil {
			return nil, "", fmt.Errorf("decode png: %w", err)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", fmt.Errorf("encode jpeg: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	default:
		// last resort: try generic image.Decode
		img, _, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			return nil, "", fmt.Errorf("not an image or unsupported format")
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", fmt.Errorf("encode jpeg: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	}
}

// แยกฟังก์ชันไว้นี่เพื่อให้ง่ายต่อการ mock ใน unit test
var httpDetectContentType = func(b []byte) string {
	// พฤติกรรมเหมือน net/http.DetectContentType แต่หลีกเลี่ยง import ตรง (จะได้ปรับเปลี่ยนง่าย)
	// ถ้าคุณโอเค import net/http ก็เปลี่ยนเป็น http.DetectContentType(b)
	return detectContentTypeCompat(b)
}

// minimal mimic ของ DetectContentType (หากอยากใช้ของจริง ให้ import "net/http" แล้วเรียก http.DetectContentType)
func detectContentTypeCompat(b []byte) string {
	// ตรวจง่ายๆ พอจับ png/jpeg ได้
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg"
	}
	return "application/octet-stream"
}

var pngDecode = func(r io.Reader) (image.Image, error) {
	return png.Decode(r)
}

func ifEmptyStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---- helpers (ต่อจาก ifEmptyStr) ----
func str(v any) string {
	if v == nil {
		return ""
	}
	// ใช้ fmt.Sprint เพื่อครอบทุกชนิดให้เป็น string
	return fmt.Sprint(v)
}
