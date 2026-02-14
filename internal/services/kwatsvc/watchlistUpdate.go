// internal/services/kwatsvc/watchlistUpdate.go
package kwatsvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/kafka"
	"klynx/internal/repo/stomongo"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	"klynx/internal/gateways/iboc/watchlist/ibface"

	minioSDK "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ================== UPDATE SERVICE ==================

func WatchlistUpdate(ctx context.Context, id string, req kwatmod.WatchlistUpdateRequest) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/kwatsvc",
		"watchlist.WatchlistUpdate",
		"kwatsvc", "WatchlistUpdate",
	)
	defer end()
	traceID := traceutil.TraceIDFromCtx(ctx)
	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log.Info().Str("id", id).Msg("✏️  start update watchlist")

	// 0) resolve id: อนุญาตทั้ง _id (ObjectID) และ idcard
	raw := strings.TrimSpace(id)
	var oid primitive.ObjectID

	if _, err := primitive.ObjectIDFromHex(raw); err == nil {
		oid, _ = primitive.ObjectIDFromHex(raw)
		log.Info().Str("id", id).Msg("🔑 resolved by _id")
	} else {
		// หาเอกสารด้วย idcard ที่ยังไม่ถูก soft-delete
		filter := bson.M{
			"idcard":    raw,
			"isDeleted": bson.M{"$ne": true}, // ✅ กัน soft-delete
		}
		var found struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := stomongo.FindOne(ctx, watchlistColl, filter, &found); err != nil || found.ID.IsZero() {
			log.Warn().Str("id_or_idcard", id).Msg("🚫 watchlist not found by _id or idcard")
			return ErrWatchlistNotFound
		}
		oid = found.ID
		// ปรับ id ให้เป็น _id.Hex() เพื่อความสม่ำเสมอในการ emit event/datalog
		id = oid.Hex()
		log.Info().Str("idcard", raw).Str("resolvedId", id).Msg("🔑 resolved by idcard → _id")
	}

	// 1) load current doc (อ่านค่าเดิมสำหรับ merge/คำนวณ rev/หา key เดิม)
	var cur bson.M
	if err := stomongo.FindOne(ctx, watchlistColl, bson.M{"_id": oid}, &cur); err != nil {
		log.Warn().Str("id", id).Msg("🚫 watchlist not found")
		return ErrWatchlistNotFound
	}

	oldKey := str(cur["photoKey"])
	oldMime := str(cur["photoContentType"])
	oldRev := int64(0)
	switch v := cur["rev"].(type) {
	case int64:
		oldRev = v
	case int32:
		oldRev = int64(v)
	}

	// อ่าน type เดิม + origin/face keys
	oldType := 0
	switch v := cur["type"].(type) {
	case int:
		oldType = v
	case int32:
		oldType = int(v)
	case int64:
		oldType = int(v)
	case float64:
		oldType = int(v)
	}
	oldOriginKey := str(cur["photoOriginKey"])
	oldFaceKey := str(cur["photoFaceKey"])

	// 2) ถ้าจะเปลี่ยน idcard → เช็ค duplicate (ยกเว้นเอกสารตัวเอง และกัน soft-delete)
	if req.IdCard != nil && strings.TrimSpace(*req.IdCard) != "" {
		idc := strings.TrimSpace(*req.IdCard)
		log.Debug().Str("idcard", idc).Msg("🔎 checking duplicate (update)")
		filterDup := bson.M{
			"idcard":    idc,
			"_id":       bson.M{"$ne": oid},
			"isDeleted": bson.M{"$ne": true}, // ✅ สำคัญ: ไม่ชนกับตัวที่ถูกลบ
		}
		var exists struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := stomongo.FindOne(ctx, watchlistColl, filterDup, &exists); err == nil && !exists.ID.IsZero() {
			log.Warn().Str("existingDocId", exists.ID.Hex()).Str("reason", "duplicateIdcard").Msg("🚫 duplicate idcard on update")
			return ErrDuplicateIDCard
		}
	}

	set := bson.M{}
	now := time.Now().UTC()
	if req.DeathStatus == nil {
		set["deathStatus"] = 1
	}
	// 3) จัดการรูปใหม่ → อัปโหลดเป็น "origin" (face ให้ handler ทำ)
	var originKey, faceKey, usedKey, usedMime, originMime string
	hasNewPhoto := false
	if len(req.PhotoFile) > 0 {
		hasNewPhoto = true

		// validate อย่างน้อยให้เป็นไฟล์รูป/มีหน้า
		jpegBytes, _, err := ensureJPEG(req.PhotoFile)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadPhoto, err)
		}
		if ib := ibface.NewFromEnv("IBOC_6"); ib == nil || !ib.Configured() {
			return fmt.Errorf("iboc not configured for crop")
		} else {
			if _, _, err := ib.AnalyzeCrop(ctx, jpegBytes); err != nil {
				if errors.Is(err, ibface.ErrNoFaceDetected) {
					return ErrNoFace
				}
				return fmt.Errorf("%w: %v", ErrBadPhoto, err)
			}
		}

		// 3.1 อัป origin
		originMime = httpDetectContentType(req.PhotoFile)
		if originMime == "" || originMime == "application/octet-stream" {
			originMime = "image/jpeg"
		}
		originKey = str(cur["photoOriginKey"])
		if originKey == "" {
			originKey = fmt.Sprintf("watchlist/%s/origin%s", id, extForMime(originMime))
		}
		if _, err := config.S3Client.PutObject(
			ctx, "kwatch", originKey,
			bytes.NewReader(req.PhotoFile), int64(len(req.PhotoFile)),
			minioSDK.PutObjectOptions{ContentType: originMime},
		); err != nil {
			return fmt.Errorf("upload origin to s3 failed: %w", err)
		}

		// 3.2 พยายามครอป → อัป face
		faceKey = str(cur["photoFaceKey"])
		if faceKey == "" {
			faceKey = fmt.Sprintf("watchlist/%s/face.jpg", id)
		}
		if ib := ibface.NewFromEnv("IBOC_6"); ib != nil && ib.Configured() {
			if cropped, mime, ce := ib.AnalyzeCrop(ctx, jpegBytes); ce == nil && len(cropped) > 0 {
				ct := strings.TrimSpace(mime)
				if ct == "" {
					ct = "image/jpeg"
				}
				if _, err := config.S3Client.PutObject(
					ctx, "kwatch", faceKey,
					bytes.NewReader(cropped), int64(len(cropped)),
					minioSDK.PutObjectOptions{ContentType: ct},
				); err == nil {
					usedKey, usedMime = faceKey, ct
					set["photoFaceKey"] = faceKey
					set["photoFaceContentType"] = ct
				}
			}
		}

		// 3.3 ตั้ง pointer แสดงผล (face ถ้าพร้อม ไม่งั้น origin)
		if usedKey == "" {
			usedKey, usedMime = originKey, originMime
		}
		set["photoOriginKey"] = originKey
		set["photoOriginContentType"] = originMime
		set["photoKey"] = usedKey
		set["photoContentType"] = usedMime

		// ถ้ายังไม่มีค่า faceKey เดิม ให้บันทึกไว้ (เผื่อ handler ใช้)
		if strings.TrimSpace(oldFaceKey) == "" {
			set["photoFaceKey"] = faceKey
		}
	}

	// 4) map ฟิลด์ pointer → $set เฉพาะที่ส่งมา (age เป็น int)
	setIntIf(set, "type", req.Type)
	setIntIf(set, "personalType", req.PersonalType)
	setIntIf(set, "crimesType", req.CrimesType)
	setStrIf(set, "idcard", req.IdCard)
	setStrIf(set, "passport", req.Passport)
	setStrIf(set, "titlename", req.TitleName)
	setStrIf(set, "subTitlename", req.SubTitleName)
	setStrIf(set, "firstname", req.FirstName)
	setStrIf(set, "lastname", req.LastName)
	setStrIf(set, "nickname", req.NickName)
	setStrIf(set, "sex", req.Sex)
	setStrIf(set, "birthday", req.Birthday)
	setIntIf(set, "age", req.Age)
	setStrIf(set, "fatherName", req.FatherName)
	setStrIf(set, "fatherIdcard", req.FatherIdCard)
	setStrIf(set, "motherName", req.MotherName)
	setStrIf(set, "motherIdcard", req.MotherIdCard)
	setStrIf(set, "maritalStatus", req.MaritalStatus)
	setIntIf(set, "deathStatus", req.DeathStatus)
	setStrIf(set, "dateOfDeath", req.DateOfDeath)
	setIntIf(set, "policeRegion", req.PoliceRegion)
	setIntIf(set, "policeProvincial", req.PoliceProvincial)
	setIntIf(set, "policeStation", req.PoliceStation)
	setStrIf(set, "userRecorder", req.UserRecorder)
	setStrIf(set, "userPosition", req.UserPosition)
	setStrIf(set, "alertType", req.AlertType)
	setStrIf(set, "alertDesc", req.AlertDesc)

	if req.Status != nil {
		set["status"] = parseBoolStr(*req.Status, true)
	}

	// 5) ตรวจ type change + validation เฉพาะกรณีเปลี่ยน
	newType := oldType
	if v, ok := set["type"]; ok {
		switch t := v.(type) {
		case int:
			newType = t
		case int32:
			newType = int(t)
		case int64:
			newType = int(t)
		case float64:
			newType = int(t)
		}
	}

	// ensureOrigin() สำหรับ typeto2 ตอน "ไม่มีรูปใหม่"
	ensureOrigin := func() error {
		if originKey != "" { // จากบล็อกรูปใหม่ หรือค่าเดิม
			return nil
		}
		// ใช้ค่าเดิมจาก DB หากมี
		originKey = strings.TrimSpace(oldOriginKey)
		if originKey != "" {
			return nil
		}

		// ถ้ายังไม่มี ให้ fallback ใช้ภาพเดิม (display) แล้ว "คัดลอก" ไปเป็น origin
		srcKey := strings.TrimSpace(oldKey)
		if srcKey == "" {
			return fmt.Errorf("typeTo2 requires a photo: no origin/photoKey found")
		}
		originKey = fmt.Sprintf("watchlist/%s/origin.jpg", id)

		// S3 copy: photoKey -> photoOriginKey
		src := minioSDK.CopySrcOptions{Bucket: "kwatch", Object: srcKey}
		dst := minioSDK.CopyDestOptions{Bucket: "kwatch", Object: originKey}
		if _, err := config.S3Client.CopyObject(ctx, dst, src); err != nil {
			return fmt.Errorf("ensure origin copy failed: %w", err)
		}

		if strings.TrimSpace(originMime) == "" {
			originMime = firstNonEmpty(oldMime, "image/jpeg")
		}
		set["photoOriginKey"] = originKey
		set["photoOriginContentType"] = originMime
		// ไม่ต้องแตะ face ที่นี่ ให้ handler typeto2 เป็นคนครอป
		return nil
	}

	typeChanged := newType != oldType
	if typeChanged {
		switch newType {
		case 2:
			// validate ตัวเลข
			v, ok := atoiOpt(choose(req.CrimesType, nil))
			if !ok {
				return fmt.Errorf("crimesType is required and must be a number when type=2")
			}
			set["type"] = newType
			set["crimesType"] = v
			set["personalType"] = nil

			// ✅ ถ้า typeto2 และ "ไม่มีรูปใหม่" ให้ ensure origin เสมอ
			if !hasNewPhoto {
				if err := ensureOrigin(); err != nil {
					return err
				}
			}

		case 1:
			v, ok := atoiOpt(choose(req.PersonalType, nil))
			if !ok {
				return fmt.Errorf("personalType is required and must be a number when type=1")
			}
			set["type"] = newType
			set["personalType"] = v
			set["crimesType"] = nil

		default:
			return fmt.Errorf("invalid type: must be 1 or 2")
		}
	}

	// 6) state → updating
	set["state"] = "updating"
	set["updatedAt"] = now

	// 7) do update (+inc rev)
	startDB := time.Now()
	if _, err := stomongo.UpdateOneSetInc(
		ctx,
		watchlistColl,
		bson.M{"_id": oid},
		set,
		bson.M{"rev": 1},
	); err != nil {
		log.Error().
			Err(err).
			Str("id", id).
			Dur("took", time.Since(startDB)).
			Msg("💥 mongo update failed")
		return fmt.Errorf("mongo update failed: %w", err)
	}
	log.Info().
		Str("id", id).
		Dur("took", time.Since(startDB)).
		Msg("✅ watchlist updated")

	// 8) emit kafka (เลือก event ให้ handler ทำงานถูกเส้นทาง)
	state := "updating"
	// ถ้าไม่มีรูปใหม่ → ใช้ค่าจากเอกสารเดิมสำหรับ event
	if originKey == "" {
		originKey = oldOriginKey
	}
	if faceKey == "" {
		faceKey = oldFaceKey
	}
	originCT := originMime
	if originCT == "" {
		originCT = str(cur["photoOriginContentType"])
	}
	evPhotoKey := firstNonEmpty(usedKey, firstNonEmpty(oldKey, oldOriginKey)) // compat
	evPhotoMime := firstNonEmpty(usedMime, oldMime)

	evtEvent := "watchlist.updated.meta"
	evtSchema := "kwatch/watchlistUpdate/2"
	if typeChanged && newType == 2 {
		evtEvent = "watchlist.updated.typeTo2" // handler: สร้าง IBOC จาก origin
	} else if typeChanged && newType == 1 {
		evtEvent = "watchlist.updated.typeTo1" // handler: ลบ IBOC
	} else if hasNewPhoto {
		evtEvent = "watchlist.updated.withFace" // handler: ใช้ face/origin ตาม key และทำ IBOC ถ้า type=2
	}

	evt := kwatmod.WatchlistEvent{
		ID:                     id,
		Event:                  evtEvent,
		TraceID:                traceID,
		Time:                   now.UTC().Format(time.RFC3339Nano),
		Rev:                    oldRev + 1,
		State:                  state,
		PhotoKey:               evPhotoKey,
		PhotoContentType:       evPhotoMime,
		PhotoOriginKey:         originKey,
		PhotoOriginContentType: originCT,
		PhotoFaceKey:           faceKey,

		// ---- INT fields (ใช้ chooseInt) ----
		Type:             chooseIntStr(req.Type, cur["type"]),
		PersonalType:     chooseIntStr(req.PersonalType, cur["personalType"]),
		CrimesType:       chooseIntStr(req.CrimesType, cur["crimesType"]),
		Age:              chooseIntStr(req.Age, cur["age"]),
		DeathStatus:      chooseIntStr(req.DeathStatus, cur["deathStatus"]),
		PoliceRegion:     chooseIntStr(req.PoliceRegion, cur["policeRegion"]),
		PoliceProvincial: chooseIntStr(req.PoliceProvincial, cur["policeProvincial"]),
		PoliceStation:    chooseIntStr(req.PoliceStation, cur["policeStation"]),

		// ---- STRING fields (ใช้ chooseStr) ----
		IdCard:        chooseStr(req.IdCard, cur["idcard"]),
		Passport:      chooseStr(req.Passport, cur["passport"]),
		TitleName:     chooseStr(req.TitleName, cur["titlename"]),
		SubTitleName:  chooseStr(req.SubTitleName, cur["subTitlename"]),
		FirstName:     chooseStr(req.FirstName, cur["firstname"]),
		LastName:      chooseStr(req.LastName, cur["lastname"]),
		NickName:      chooseStr(req.NickName, cur["nickname"]),
		Sex:           chooseStr(req.Sex, cur["sex"]),
		Birthday:      chooseStr(req.Birthday, cur["birthday"]),
		FatherName:    chooseStr(req.FatherName, cur["fatherName"]),
		FatherIdCard:  chooseStr(req.FatherIdCard, cur["fatherIdcard"]),
		MotherName:    chooseStr(req.MotherName, cur["motherName"]),
		MotherIdCard:  chooseStr(req.MotherIdCard, cur["motherIdcard"]),
		MaritalStatus: chooseStr(req.MaritalStatus, cur["maritalStatus"]),
		DateOfDeath:   chooseStr(req.DateOfDeath, cur["dateOfDeath"]),
		UserRecorder:  chooseStr(req.UserRecorder, cur["userRecorder"]),
		UserPosition:  chooseStr(req.UserPosition, cur["userPosition"]),
		AlertType:     chooseStr(req.AlertType, cur["alertType"]),
		AlertDesc:     chooseStr(req.AlertDesc, cur["alertDesc"]),
	}

	headers := map[string]string{
		"event":   evt.Event,
		"source":  "kwatsvc",
		"schema":  evtSchema,
		"traceId": traceID, // keep human-friendly traceId ด้วย
	}
	const topicWatchlist = "kwatch.watchlist"
	if err := kafka.PublishEventTo(ctx, topicWatchlist, id, evt, headers); err != nil {
		log.Error().Err(err).Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_failed")
	} else {
		log.Info().Str("docId", evt.ID).Str("topic", topicWatchlist).Msg("kafka_emit_ok")
	}

	log.Info().Str("id", id).Msg("🎉 update watchlist done")
	return nil
}
