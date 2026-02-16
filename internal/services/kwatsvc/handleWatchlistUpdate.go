// internal/services/kwatsvc/handleWatchlistUpdate.go
package kwatsvc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/iboc/watchlist/ibface"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kwatchmsg"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func HandleWatchlistUpdate(parent context.Context, evt kwatmod.WatchlistEvent, g Gateways) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"github.com/hotkhwan/gateway-api/kwatsvc",
		"watchlist.HandleWatchlistUpdate",
		"kwatsvc", "HandleWatchlistUpdate",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Info().
		Str("watchlistID", evt.ID).
		Str("event", strings.ToLower(evt.Event)).
		Str("state", strings.ToLower(evt.State)).
		Msg("handler_start")

	var objID primitive.ObjectID
	if oid, err := primitive.ObjectIDFromHex(evt.ID); err == nil {
		objID = oid
	} else {
		log.Warn().Err(err).Str("watchlistID", evt.ID).Msg("⚠️ invalid object id (will skip some DB updates)")
	}

	oldFaceProd, oldPersonProd := extractIDsFromEvtExternal(evt.External, "iboc")
	oldFaceDev, oldPersonDev := extractIDsFromEvtExternal(evt.External, "ibocdev")
	if (oldFaceProd == "" || oldPersonProd == "" || oldFaceDev == "" || oldPersonDev == "") && objID != primitive.NilObjectID {
		var cur struct {
			External map[string]any `bson:"external"`
		}
		_ = config.DB.Collection(watchlistColl).FindOne(ctx, bson.M{"_id": objID}).Decode(&cur)
		if f, p := extractIDsFromEvtExternal(cur.External, "iboc"); oldFaceProd == "" || oldPersonProd == "" {
			if oldFaceProd == "" {
				oldFaceProd = f
			}
			if oldPersonProd == "" {
				oldPersonProd = p
			}
		}
		if f, p := extractIDsFromEvtExternal(cur.External, "ibocdev"); oldFaceDev == "" || oldPersonDev == "" {
			if oldFaceDev == "" {
				oldFaceDev = f
			}
			if oldPersonDev == "" {
				oldPersonDev = p
			}
		}
	}

	isType2 := evt.Type == 2
	isWithFace := strings.EqualFold(evt.Event, "watchlist.updated.withface")

	downloadAndMaybeCrop := func(key string, doCrop bool) (out []byte, contentType string, err error) {
		if strings.TrimSpace(key) == "" {
			return nil, "", fmt.Errorf("empty key")
		}
		pb, _, e := stos3minio.DownloadByKey(ctx, "kwatch", key)
		if e != nil || len(pb) == 0 {
			return nil, "", fmt.Errorf("download failed: %w", e)
		}
		jb, _, je := ensureJPEG(pb)
		src := jb
		if je != nil || len(jb) == 0 {
			src = pb
		}
		if !doCrop {
			return src, http.DetectContentType(src), nil
		}
		ib := ibface.NewFromEnv("IBOC_6")
		if ib == nil || !ib.Configured() {
			return nil, "", fmt.Errorf("iboc_6 not configured for crop")
		}
		cb, mime, ce := ib.AnalyzeCrop(ctx, src)
		if ce != nil || len(cb) == 0 {
			return nil, "", fmt.Errorf("crop failed: %w", ce)
		}
		ct := strings.TrimSpace(mime)
		if ct == "" {
			ct = "image/jpeg"
		}
		return cb, ct, nil
	}

	// ---------- Helper: fallback station title จาก DB ----------
	getFirstWarrantStationTitle := func(ctx context.Context, id primitive.ObjectID) string {
		if id == primitive.NilObjectID {
			return ""
		}
		var cur struct {
			Warrants []struct {
				PoliceStation string `bson:"policeStation"`
			} `bson:"warrants"`
		}
		_ = config.DB.Collection(watchlistColl).
			FindOne(ctx, bson.M{"_id": id}).
			Decode(&cur)
		if len(cur.Warrants) > 0 {
			return strings.TrimSpace(cur.Warrants[0].PoliceStation)
		}
		return ""
	}

	// =========================
	// Watchman Upsert (เสมอ)
	// =========================
	if g.Watchman == nil || !g.Watchman.Configured() {
		log.Info().Str("watchlistID", evt.ID).Msg("↷ Skip Watchman: gateway not configured")
	} else if strings.TrimSpace(evt.IdCard) == "" {
		log.Info().Str("watchlistID", evt.ID).Msg("↷ Skip Watchman: empty idcard")
	} else {
		fieldsMap := buildWatchmanFieldsMap(evt) // ← map แบบสะอาด ไม่มี "<nil>"

		// เลือกรูป: face.jpg ถ้ามี ไม่งั้น origin.jpg
		var photoToSend []byte
		photoFileName := "origin.jpg"
		sendPhoto := false

		if isWithFace && strings.TrimSpace(evt.PhotoKey) != "" && strings.Contains(evt.PhotoKey, "/face") {
			if pb, _, e := stos3minio.DownloadByKey(ctx, "kwatch", evt.PhotoKey); e == nil && len(pb) > 0 {
				if jb, _, je := ensureJPEG(pb); je == nil && len(jb) > 0 {
					photoToSend = jb
				} else {
					photoToSend = pb
				}
				photoFileName = "face.jpg"
				sendPhoto = true
			}
		}
		if !sendPhoto {
			src := strings.TrimSpace(evt.PhotoOriginKey)
			if src == "" && objID != primitive.NilObjectID {
				var cur struct {
					PhotoOriginKey string `bson:"photoOriginKey"`
				}
				_ = config.DB.Collection(watchlistColl).FindOne(ctx, bson.M{"_id": objID}).Decode(&cur)
				src = cur.PhotoOriginKey
			}
			if src != "" {
				if pb, _, e := stos3minio.DownloadByKey(ctx, "kwatch", src); e == nil && len(pb) > 0 {
					if jb, _, je := ensureJPEG(pb); je == nil && len(jb) > 0 {
						photoToSend = jb
					} else {
						photoToSend = pb
					}
					photoFileName = "origin.jpg"
					sendPhoto = true
				}
			}
		}

		now := time.Now().UTC()
		wmID, status, err := g.Watchman.UpsertThenEnsureID(ctx, fieldsMap, photoFileName, photoToSend)
		if err != nil || status >= 400 || wmID <= 0 {
			if objID != primitive.NilObjectID {
				_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": objID}, bson.M{
					"external.watchman.id":       "",
					"external.watchman.state":    "error",
					"external.watchman.syncedAt": now,
					"external.watchman.note":     fmt.Sprintf("create/upsert failed: %v (status=%d)", err, status),
					"updatedAt":                  now,
				})
			}
			log.Warn().Err(err).Int("status", status).Str("watchlistID", evt.ID).Bool("sendPhoto", sendPhoto).Msg("⚠️ Watchman upsert failed")
		} else {
			if objID != primitive.NilObjectID {
				_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": objID}, bson.M{
					"external.watchman.id":       fmt.Sprintf("%d", wmID),
					"external.watchman.state":    "active",
					"external.watchman.syncedAt": time.Now().UTC(),
					"updatedAt":                  time.Now().UTC(),
				})
			}
			log.Info().Str("watchlistID", evt.ID).Int64("watchmanID", wmID).Msg("✅ Watchman created/updated")
		}
	}

	switch strings.ToLower(evt.Event) {
	case "watchlist.updated.typeto1":
		{
			for _, t := range []struct {
				name string
				cli  *ibface.Client
				fid  string
				pid  string
			}{
				{"iboc", g.IBOCProd, oldFaceProd, oldPersonProd},
				{"ibocdev", g.IBOCDev, oldFaceDev, oldPersonDev},
			} {
				if t.cli == nil || !t.cli.Configured() {
					continue
				}
				// if strings.TrimSpace(t.fid) != "" {
				// 	_ = t.cli.DeleteFace(ctx, t.fid)
				// }
				if strings.TrimSpace(t.pid) != "" {
					reason := strings.TrimSpace(evt.UserRecorder)
					if reason == "" {
						reason = "kwatsvc:update:type!=2:" + evt.ID
					}
					_ = t.cli.DeletePerson(ctx, t.pid, reason)
				}
				if objID != primitive.NilObjectID {
					empty := ""
					_ = upsertExternal(ctx, t.name, objID, "", &empty, "deleted")
				}
			}
		}

	case "watchlist.updated.typeto2", "watchlist.updated.withface":
		{
			origin := strings.TrimSpace(evt.PhotoOriginKey)
			if origin == "" {
				origin = strings.TrimSpace(evt.PhotoKey)
			}
			if origin == "" && objID != primitive.NilObjectID {
				var cur struct {
					PhotoOriginKey string `bson:"photoOriginKey"`
				}
				_ = config.DB.Collection(watchlistColl).FindOne(ctx, bson.M{"_id": objID}).Decode(&cur)
				origin = strings.TrimSpace(cur.PhotoOriginKey)
			}
			if origin == "" {
				log.Warn().Str("watchlistID", evt.ID).Msg("⚠️ withFace/typeTo2 but no origin key")
				break
			}
			faceKey := strings.TrimSpace(evt.PhotoFaceKey)
			if faceKey == "" {
				faceKey = fmt.Sprintf("watchlist/%s/face.jpg", evt.ID)
			}
			cropped, ct, err := downloadAndMaybeCrop(origin, true)
			if err != nil {
				if strings.EqualFold(strings.TrimSpace(evt.Event), "watchlist.updated.typeto2") {
					return fmt.Errorf("typeTo2: %w", err)
				}
				return fmt.Errorf("withFace: %w", err)
			}
			log.Info().Str("key", faceKey).Int("bytes", len(cropped)).Str("ct", ct).Msg("⬆️ wrote CROPPED face to S3")

			if isType2 {
				alertTypeID := strings.TrimSpace(evt.AlertType)
				alertDesc := strings.TrimSpace(evt.AlertDesc)
				alertTitle, _ := resolveAlertTitle(ctx, alertTypeID)
				crimesTitle, _ := resolveCrimesTitle(ctx, evt.CrimesType)

				// (NEW) สร้าง stationTitle โดย fallback เมื่อไม่มี int code
				var stationTitle string
				if evt.PoliceStation > 0 {
					stationTitle, _ = resolvePoliceStationTitle(ctx, evt.PoliceProvincial, evt.PoliceStation)
				} else {
					st := strings.TrimSpace(evt.StationTitleFallback)
					if st == "" && objID != primitive.NilObjectID {
						st = getFirstWarrantStationTitle(ctx, objID)
					}
					stationTitle = st
				}

				originBytes, _, derr := stos3minio.DownloadByKey(ctx, "kwatch", origin)
				if derr != nil || len(originBytes) == 0 {
					log.Warn().Err(derr).Str("origin", origin).Msg("⚠️ cannot download origin for IBOC (skip)")
				} else {
					for _, t := range []struct {
						name      string
						cli       *ibface.Client
						oldFaceID string
						personID  string
					}{
						{"iboc", g.IBOCProd, oldFaceProd, oldPersonProd},
						{"ibocdev", g.IBOCDev, oldFaceDev, oldPersonDev},
					} {
						if t.cli == nil || !t.cli.Configured() {
							continue
						}
						pid := strings.TrimSpace(t.personID)
						// if strings.TrimSpace(t.oldFaceID) != "" {
						// 	_ = t.cli.DeleteFace(ctx, t.oldFaceID)
						// }
						if pid == "" {
							pp, e := t.cli.EnsurePerson(ctx,
								evt.FirstName, evt.LastName, evt.IdCard,
								alertTitle, alertDesc, crimesTitle, stationTitle)
							if e != nil {
								log.Warn().Err(e).Str("ns", t.name).Msg("⚠️ ensure person failed; skip")
								continue
							}
							pid = pp
						}
						newFaceID, e := t.cli.AttachFaceFromOriginal(ctx, pid, originBytes, evt.ID)
						if e != nil {
							log.Warn().Err(e).Str("ns", t.name).Str("personId", pid).Msg("⚠️ attach face failed")
							_ = t.cli.UpdatePerson(ctx,
								pid, evt.FirstName, evt.LastName, evt.IdCard,
								alertTitle, alertDesc, crimesTitle, stationTitle)
							if objID != primitive.NilObjectID {
								_ = upsertExternal(ctx, t.name, objID, pid, nil, "active")
							}
							continue
						}
						_ = t.cli.UpdatePerson(ctx,
							pid, evt.FirstName, evt.LastName, evt.IdCard,
							alertTitle, alertDesc, crimesTitle, stationTitle)
						if objID != primitive.NilObjectID {
							_ = upsertExternal(ctx, t.name, objID, pid, &newFaceID, "active")
						}
						log.Info().Str("ns", t.name).Str("personId", pid).Str("faceId", newFaceID).Msg("✅ IBOC updated face")
					}
				}
			}
		}

	case "watchlist.updated.meta":
		{
			alertTypeID := strings.TrimSpace(evt.AlertType)
			alertDesc := strings.TrimSpace(evt.AlertDesc)
			alertTitle, _ := resolveAlertTitle(ctx, alertTypeID)
			crimesTitle, _ := resolveCrimesTitle(ctx, evt.CrimesType)

			// (NEW) คำนวณ stationTitle โดย fallback
			var stationTitle string
			if evt.PoliceStation > 0 {
				stationTitle, _ = resolvePoliceStationTitle(ctx, evt.PoliceProvincial, evt.PoliceStation)
			} else {
				st := strings.TrimSpace(evt.StationTitleFallback)
				if st == "" && objID != primitive.NilObjectID {
					st = getFirstWarrantStationTitle(ctx, objID)
				}
				stationTitle = st
			}

			// ถ้า type=2 และ "external ไม่มี" ให้สร้าง person ใหม่ทันที (แม้เป็น meta)
			if isType2 {
				for _, t := range []struct {
					name string
					cli  *ibface.Client
					pid  string
				}{
					{"iboc", g.IBOCProd, oldPersonProd},
					{"ibocdev", g.IBOCDev, oldPersonDev},
				} {
					if t.cli == nil || !t.cli.Configured() {
						continue
					}
					pid := strings.TrimSpace(t.pid)
					if pid == "" {
						pp, e := t.cli.EnsurePerson(ctx,
							evt.FirstName, evt.LastName, evt.IdCard,
							alertTitle, alertDesc, crimesTitle, stationTitle)
						if e != nil {
							log.Warn().Err(e).Str("ns", t.name).Msg("⚠️ ensure person (meta) failed; skip")
							continue
						}
						pid = pp
						if objID != primitive.NilObjectID {
							_ = upsertExternal(ctx, t.name, objID, pid, nil, "active")
						}
						log.Info().Str("ns", t.name).Str("personId", pid).Msg("✅ IBOC person created (meta)")
					} else {
						_ = t.cli.UpdatePerson(ctx, pid, evt.FirstName, evt.LastName, evt.IdCard,
							alertTitle, alertDesc, crimesTitle, stationTitle)
						if objID != primitive.NilObjectID {
							_ = upsertExternal(ctx, t.name, objID, pid, nil, "active")
						}
						log.Info().Str("ns", t.name).Str("personId", pid).Msg("✅ IBOC person updated (metadata)")
					}
				}
			}
		}
	}

	// Final state
	if objID != primitive.NilObjectID {
		now := time.Now().UTC()
		_, _ = stomongo.UpdateOne(ctx, watchlistColl,
			bson.M{"_id": objID},
			bson.M{
				"state":     "updated",
				"updatedAt": now,
			},
		)
		log.Info().Str("watchlistID", evt.ID).Msg("📌 Watchlist state set to UPDATED")
	}
	if err := kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.updated", "msg data update is done"); err != nil {
		log.Error().Err(err).Msg("❌ Failed to publish public updated event")
	}
	log.Info().Str("watchlistID", evt.ID).Msg("✅ Watchlist UPDATE synced (end)")
	return nil
}

// ใส่ไว้ด้านบนไฟล์ (ใกล้ ๆ imports)
func normalizeSexForWatchman(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "male", "m", "ชาย":
		return "ชาย"
	case "female", "f", "หญิง":
		return "หญิง"
	default:
		return s // ถ้าส่งมาแล้วเป็นไทยอยู่ ก็ปล่อยผ่าน
	}
}

// สร้าง map สำหรับ Watchman แบบ "omit empty" และไม่มี "<nil>"
func buildWatchmanFieldsMap(evt kwatmod.WatchlistEvent) map[string]string {
	m := map[string]string{}
	put := func(k, v string) {
		v = strings.TrimSpace(v)
		if v == "" || v == "<nil>" || strings.EqualFold(v, "null") {
			return
		}
		m[k] = v
	}
	putInt := func(k string, v int) {
		if v <= 0 {
			return
		}
		m[k] = utils.Itoa(v)
	}

	// === minimum required ตามตัวอย่าง cURL ===
	put("type", utils.Itoa(evt.Type))                       // "2"
	put("crimesType", utils.Itoa(evt.CrimesType))           // "1"
	put("idcard", evt.IdCard)                               // "1234567890121"
	put("titlename", evt.TitleName)                         // "นาย"
	put("firstname", evt.FirstName)                         // "ชื่อทดสอบ"
	put("lastname", evt.LastName)                           // "นามสกุลทดสอบ"
	put("sex", normalizeSexForWatchman(evt.Sex))            // "ชาย"/"หญิง"
	put("deathStatus", utils.Itoa(max(1, evt.DeathStatus))) // บังคับอย่างน้อย "1"

	// === optional (ส่งเมื่อมีค่าเท่านั้น) ===
	put("subTitlename", evt.SubTitleName)
	put("nickname", evt.NickName)
	put("birthday", evt.Birthday)
	if evt.Age > 0 {
		put("age", utils.Itoa(evt.Age))
	}
	put("fatherName", evt.FatherName)
	put("fatherIdcard", evt.FatherIdCard)
	put("motherName", evt.MotherName)
	put("motherIdcard", evt.MotherIdCard)
	put("maritalStatus", evt.MaritalStatus)
	put("dateOfDeath", evt.DateOfDeath)
	put("passport", evt.Passport)

	// ตำรวจ: ส่งเฉพาะ code (int) ตามสเปก
	putInt("policeRegion", evt.PoliceRegion)
	putInt("policeProvincial", evt.PoliceProvincial)
	putInt("policeStation", evt.PoliceStation)

	// ผู้บันทึก
	put("userRecorder", evt.UserRecorder)
	put("userPosition", evt.UserPosition)

	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
