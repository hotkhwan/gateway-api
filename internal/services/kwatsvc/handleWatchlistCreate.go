// internal/services/kwatsvc/handleWatchlistCreate.go
package kwatsvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/gateways/iboc/watchlist/ibface"
	"klynx/internal/mqtt/kwatchmsg"
	"klynx/internal/repo/stomongo"
	"klynx/internal/repo/stos3minio"
	"klynx/models/kwatmod"
	"klynx/utils"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	minio "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------- สำหรับ event: watchlist.created ----------
func HandleWatchlistCreate(parent context.Context, evt kwatmod.WatchlistEvent, g Gateways) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"klynx/kwatsvc",                   // tracerName
		"watchlist.HandleWatchlistCreate", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kwatsvc", "HandleWatchlistCreate",
	)
	defer end()

	// ใช้ ctx เดียว แล้วค่อย wrap timeout
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Info().
		Str("watchlistID", evt.ID).
		Str("event", strings.ToLower(evt.Event)).
		Str("state", strings.ToLower(evt.State)).
		Msg("handler_start")

	// objID (ไว้ทำความสะอาด/อัปเดต)
	var objID primitive.ObjectID
	if oid, err := primitive.ObjectIDFromHex(evt.ID); err == nil {
		objID = oid
	} else {
		log.Warn().Err(err).Str("watchlistID", evt.ID).Msg("⚠️ invalid object id (will skip DB updates)")
	}

	// ---------------------------
	// 1) Prepare photo ดาวน์โหลดรูปต้นฉบับจาก S3
	// ---------------------------
	var photoData []byte
	originKey := strings.TrimSpace(evt.PhotoOriginKey)
	if originKey == "" {
		// เพื่อรองรับ event เก่า: fallback ไปใช้ PhotoKey
		originKey = strings.TrimSpace(evt.PhotoKey)
	}
	if originKey == "" {
		log.Info().Str("watchlistID", evt.ID).Msg("↷ Skip image ops: no origin key")
	} else {
		if err := httputil.Retry(ctx,
			httputil.RetryCfg{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second, Jitter: true},
			func(attempt int) (bool, error) {
				start := time.Now()
				p, _, e := stos3minio.DownloadByKey(ctx, "kwatch", originKey)
				if e != nil {
					log.Warn().Err(e).
						Str("watchlistID", evt.ID).
						Str("originKey", originKey).
						Int("attempt", attempt).
						Msg("⚠️ Download origin failed, will retry if retryable")
				}
				photoData = p
				log.Debug().
					Str("watchlistID", evt.ID).
					Str("originKey", originKey).
					Int("bytes", len(photoData)).
					Dur("took", time.Since(start)).
					Msg("🖼️ Origin downloaded from S3")
				return e != nil && httputil.RetryableErr(e), e
			},
		); err != nil {
			log.Error().Err(err).Str("watchlistID", evt.ID).Msg("❌ failed to download origin from S3")
		}
	}

	// normalize → JPEG
	jpegBytes := photoData
	if b, _, e := ensureJPEG(photoData); e == nil && len(b) > 0 {
		jpegBytes = b
	} else if e != nil {
		log.Warn().Err(e).Str("watchlistID", evt.ID).Msg("⚠️ ensureJPEG failed → use original bytes")
	}

	// 1.2) Crop แล้วอัปโหลดไป faceKey (ห้าม overwrite origin)
	var croppedBytes []byte
	var croppedMime string

	faceKey := strings.TrimSpace(evt.PhotoFaceKey) // ควรได้มาจาก event (ถ้าไม่มี ให้ตั้ง default)
	if faceKey == "" {
		faceKey = fmt.Sprintf("watchlist/%s/face.jpg", evt.ID)
	}

	if len(jpegBytes) > 0 {
		iboc6 := ibface.NewFromEnv("IBOC_6")
		if !iboc6.Configured() {
			return fmt.Errorf("iboc not configured for crop")
		}
		cb, mime, err := iboc6.AnalyzeCrop(ctx, jpegBytes)
		if err != nil {
			if errors.Is(err, ibface.ErrNoFaceDetected) {
				return fmt.Errorf("%w", ErrNoFace)
			}
			return fmt.Errorf("%w: %v", ErrBadPhoto, err)
		}
		if len(cb) == 0 {
			return fmt.Errorf("%w: empty crop", ErrBadPhoto)
		}
		croppedBytes = cb
		if strings.TrimSpace(mime) == "" {
			croppedMime = "image/jpeg"
		} else {
			croppedMime = mime
		}

		// upload to faceKey (ไม่แตะ origin)
		const bucket = "kwatch"
		reader := bytes.NewReader(croppedBytes)
		log.Info().Str("bucket", bucket).Str("key", faceKey).Int("bytes", len(croppedBytes)).Str("ct", croppedMime).Msg("⬆️ Upload CROPPED face to S3")
		if _, err := config.S3Client.PutObject(ctx, bucket, faceKey, reader, int64(len(croppedBytes)), minio.PutObjectOptions{ContentType: croppedMime}); err != nil {
			log.Error().Err(err).Str("key", faceKey).Msg("❌ Upload face to S3 failed")
			return err
		}
	}

	// ---------------------------
	// 2) WATCHMAN UPSERT — ต้องสำเร็จเท่านั้น
	// ---------------------------
	if g.Watchman == nil || !g.Watchman.Configured() || strings.TrimSpace(evt.IdCard) == "" {
		_ = cleanupOnCreateFail(ctx, objID, strings.TrimSpace(evt.PhotoKey))
		return fmt.Errorf("watchman: not configured or idcard empty")
	}

	fields := map[string]string{
		"type":             utils.Itoa(evt.Type),
		"personalType":     utils.Itoa(evt.PersonalType),
		"crimesType":       utils.Itoa(evt.CrimesType),
		"idcard":           evt.IdCard,
		"passport":         evt.Passport,
		"titlename":        evt.TitleName,
		"subTitlename":     evt.SubTitleName,
		"firstname":        evt.FirstName,
		"lastname":         evt.LastName,
		"nickname":         evt.NickName,
		"sex":              evt.Sex,
		"birthday":         evt.Birthday,
		"age":              utils.Itoa(evt.Age),
		"fatherName":       evt.FatherName,
		"fatherIdcard":     evt.FatherIdCard,
		"motherName":       evt.MotherName,
		"motherIdcard":     evt.MotherIdCard,
		"maritalStatus":    evt.MaritalStatus,
		"deathStatus":      utils.Itoa(evt.DeathStatus),
		"dateOfDeath":      evt.DateOfDeath,
		"policeRegion":     utils.Itoa(evt.PoliceRegion),
		"policeProvincial": utils.Itoa(evt.PoliceProvincial),
		"policeStation":    utils.Itoa(evt.PoliceStation),
		"userRecorder":     evt.UserRecorder,
		"userPosition":     evt.UserPosition,
		"source":           "internal",
	}

	// เลือกรูปที่จะส่ง
	var photoToSend []byte
	var photoFileName string
	switch {
	case len(croppedBytes) > 0:
		photoToSend = croppedBytes
		photoFileName = filepath.Base(strings.TrimSpace(evt.PhotoKey))
	case len(jpegBytes) > 0:
		photoToSend = jpegBytes
		photoFileName = filepath.Base(strings.TrimSpace(evt.PhotoKey))
	}

	now := time.Now().UTC()
	wmID, status, wmErr := g.Watchman.UpsertThenEnsureID(ctx, fields, photoFileName, photoToSend)
	if wmErr != nil {
		if objID != primitive.NilObjectID {
			_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": objID}, bson.M{
				"external.watchman.id":       "",
				"external.watchman.idCard":   "",
				"external.watchman.state":    "error",
				"external.watchman.syncedAt": now,
				"updatedAt":                  now,
			})
		}
		_ = cleanupOnCreateFail(ctx, objID, strings.TrimSpace(evt.PhotoKey))
		return fmt.Errorf("watchman upsert failed (http %d): %v", status, wmErr)
	}
	if objID != primitive.NilObjectID {
		_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": objID}, bson.M{
			"external.watchman.id":       fmt.Sprintf("%v", wmID),
			"external.watchman.idCard":   evt.IdCard,
			"external.watchman.state":    "active",
			"external.watchman.syncedAt": now,
			"updatedAt":                  now,
		})
	}

	// ---------------------------
	// 3) IBOC (PROD & DEV) — ทำเมื่อ evt.Type == 2
	// ---------------------------
	if evt.Type == 2 {
		alertTypeID := strings.TrimSpace(evt.AlertType)
		alertDesc := strings.TrimSpace(evt.AlertDesc)

		alertTitle, _ := resolveAlertTitle(ctx, alertTypeID)      // string
		crimesTitle, _ := resolveCrimesTitle(ctx, evt.CrimesType) // int → title
		stationTitle, _ := resolvePoliceStationTitle(ctx, evt.PoliceProvincial, evt.PoliceStation)
		type ibocTarget struct {
			name string
			cli  *ibface.Client
		}
		ibocs := []ibocTarget{
			{name: "iboc", cli: g.IBOCProd},
			{name: "ibocdev", cli: g.IBOCDev},
		}
		for _, t := range ibocs {
			if t.cli == nil || !t.cli.Configured() {
				log.Info().Str("ns", t.name).Msg("↷ Skip IBOC: gateway not configured")
				continue
			}
			if len(jpegBytes) == 0 {
				log.Info().Str("ns", t.name).Msg("↷ Skip IBOC: no image bytes")
				continue
			}
			personID, err := t.cli.EnsurePerson(ctx,
				evt.FirstName, evt.LastName, evt.IdCard,
				alertTitle, alertDesc, crimesTitle, stationTitle)
			if err != nil {
				log.Warn().Err(err).Str("ns", t.name).Msg("⚠️ IBOC ensure person failed; skip this ns")
				continue
			}
			faceID, err := t.cli.AttachFaceFromOriginal(ctx, personID, jpegBytes, evt.ID)
			if err != nil {
				log.Warn().Err(err).
					Str("ns", t.name).
					Str("personId", personID).
					Msg("⚠️ IBOC attach face from origin failed; try cropped fallback")

				if len(croppedBytes) > 0 {
					faceID, err = t.cli.AttachFace(ctx, personID, croppedBytes, croppedMime, evt.ID)
					if err != nil {
						log.Warn().Err(err).
							Str("ns", t.name).
							Str("personId", personID).
							Msg("⚠️ IBOC attach face (cropped fallback) failed")
						continue
					}
					log.Info().
						Str("ns", t.name).
						Str("personId", personID).
						Str("faceId", faceID).
						Msg("✅ IBOC face attached (cropped fallback)")
				} else {
					continue
				}
			}

			if objID != primitive.NilObjectID {
				if err := upsertExternal(ctx, t.name, objID, personID, &faceID, "active"); err != nil {
					log.Warn().Err(err).Str("ns", t.name).Msg("⚠️ save external create failed")
				}
			}
		}
	} else {
		log.Info().Str("watchlistID", evt.ID).Int("type", evt.Type).Msg("↷ Skip IBOC: type != 2")
	}

	// ---------------------------
	// 4) UPDATE FINAL STATE = queued
	// ---------------------------
	if objID != primitive.NilObjectID {
		now := time.Now().UTC()
		update := bson.M{
			"state":     "active",
			"isDeleted": false,
			"updatedAt": now,

			// ชี้ photoKey ไปที่ face (สำหรับ UI ใช้รูปครอป)
			"photoKey":         faceKey,
			"photoContentType": croppedMime,

			// เก็บสองคีย์แยกชัดเจน
			"photoFaceKey":         faceKey,
			"photoFaceContentType": croppedMime,
		}
		// ถ้ามี origin ยังไม่เคยบันทึก (กรณี event เก่า)
		if originKey != "" {
			update["photoOriginKey"] = originKey
			if ct := strings.TrimSpace(evt.PhotoOriginContentType); ct != "" {
				update["photoOriginContentType"] = ct
			}
		}
		_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": objID}, update)
		log.Info().Str("watchlistID", evt.ID).Msg("📌 Watchlist state set to ACTIVE & photo keys updated")
	}
	// ยิง public mqtt
	if err := kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.created", "msg data create is done"); err != nil {
		log.Error().Err(err).Msg("❌ Failed to publish public created event")
	}
	// if err := kwatchmsg.PublicEvent("watchlist.created"); err != nil {
	// 	log.Error().Err(err).Msg("❌ Failed to publish public created event")
	// }

	log.Info().Str("watchlistID", evt.ID).Msg("✅ Watchlist CREATE synced (end)")
	return nil
}
