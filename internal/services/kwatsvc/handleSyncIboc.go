// internal/services/kwatsvc/handleSyncIboc.go
package kwatsvc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/gateways/iboc/watchlist/ibface"
	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/mqtt/kwatchmsg"
	"klynx/internal/repo/stomongo"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	minioSDK "github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NOTE: ที่อื่นใน package นี้ควรมีตัวแปร watchlistColl = "kwatch_watchlist" อยู่แล้ว
// ถ้ายังไม่มี ให้ประกาศ: const watchlistColl = "kwatch_watchlist"

// ---------- HTTP pooled client (keep-alive) ----------
var pooledHTTP = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// HandleSyncIbocTask: consume batch task & process loop (PROD: IBOC_6)
func HandleSyncIbocTask(parent context.Context, evt kwatmod.WatchlistEvent) error {
	ctx, end, log := traceutil.StartLite(parent, "klynx/kwatsvc", "kwatsvc.HandleSyncIbocTask", "kwatsvc", "HandleSyncIbocTask")
	defer end()

	traceID := strings.TrimSpace(evt.TraceID)
	if traceID == "" {
		traceID = traceutil.TraceIDFromCtx(ctx)
	}
	log = logger.FromCtx(ctx, "kwatsvc", "HandleSyncIbocTask").With().Str("traceID", traceID).Logger()

	// Parse payload
	task, _ := evt.External["task"].(map[string]any)
	itemsAny, _ := task["items"].([]any)
	if len(itemsAny) == 0 {
		log.Info().Msg("no items in task, skip")
		return nil
	}

	ibc := ibface.NewFromEnv("IBOC_6")
	if !ibc.Configured() {
		return fmt.Errorf("iboc not configured")
	}

	sum := kwatmod.SyncIbocSummary{}
	const bulkSize = 200
	writeModels := make([]mongo.WriteModel, 0, bulkSize)

	flush := func() {
		if len(writeModels) == 0 {
			return
		}
		_, _ = stomongo.BulkWrite(ctx, watchlistColl, writeModels, options.BulkWrite().SetOrdered(false))
		writeModels = writeModels[:0]
	}

	for _, raw := range itemsAny {
		m, _ := raw.(map[string]any)
		docID := strings.TrimSpace(str(m["id"]))
		idcard := strings.TrimSpace(str(m["idcard"]))
		sum.Total++

		if docID == "" || idcard == "" {
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "empty id or idcard"})
			continue
		}
		oid, _ := primitive.ObjectIDFromHex(docID)

		// 1) ค้นหาที่ IBOC
		personID, faceID, imagePath, _, err := ibc.GetFaceByIdcard(ctx, idcard)
		if err != nil {
			// mark not_found เพื่อกันวนซ้ำรอบหน้า
			_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": oid}, bson.M{
				"external.iboc.state":    "not_found",
				"external.iboc.syncedAt": time.Now().UTC(),
			})
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: err.Error()})
			continue
		}

		// 1.5) ⬅️ NEW: update external.iboc ก่อนเลย (เรามี person/face แล้ว)
		nowPre := time.Now().UTC()
		_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": oid}, bson.M{
			"external.iboc.id":       personID,
			"external.iboc.faceId":   faceID,
			"external.iboc.state":    "active",
			"external.iboc.syncedAt": nowPre,
			"updatedAt":              nowPre,
		})

		// 1.6) ⬅️ NEW: ผลัก metadata ล่าสุดกลับ IBOC ด้วย personID ที่มี (async, non-blocking)
		{
			// ป้องกัน ctx หลักโดน cancel: ใช้ background + timeout ของตัวเอง
			mctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := syncMetaBackToIBOC(mctx, ibc, personID, oid); err != nil {
				log.Warn().Err(err).Str("docID", docID).Str("personID", personID).Msg("⚠️ syncMetaBackToIBOC failed (prod)")
			} else {
				log.Info().Str("docID", docID).Str("personID", personID).Msg("✅ IBOC metadata updated (prod)")
			}
		}

		// 2) ดาวน์โหลดรูปจาก storage
		storageURL := faceStorageAbsoluteURL(ibc.BaseURL, imagePath)
		if storageURL == "" {
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "empty storage url"})
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, storageURL, nil)
		req.Header.Set("Accept", "image/*")
		resp, gErr := pooledHTTP.Do(req)
		if gErr != nil {
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "download image failed"})
			continue
		}
		if resp == nil || resp.StatusCode >= 400 {
			if resp != nil {
				resp.Body.Close()
			}
			sum.Skipped++
			reason := "download image failed"
			if resp != nil {
				reason = fmt.Sprintf("http %d downloading image", resp.StatusCode)
			}
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: reason})
			continue
		}
		imgBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "read image failed"})
			continue
		}

		// 3) อัปโหลด S3: put origin → server-side copy เป็น face
		mime := detectContentTypeLocal(imgBytes)
		ext := extForMimeLocal(mime)
		originKey := fmt.Sprintf("watchlist/%s/origin%s", docID, ext)
		faceKey := fmt.Sprintf("watchlist/%s/face%s", docID, ext)

		// put origin
		if _, err := config.S3Client.PutObject(ctx, "kwatch", originKey, bytes.NewReader(imgBytes), int64(len(imgBytes)), minioSDK.PutObjectOptions{ContentType: mime}); err != nil {
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "s3 upload origin failed"})
			continue
		}
		// copy → face
		if _, err := config.S3Client.CopyObject(ctx,
			minioSDK.CopyDestOptions{Bucket: "kwatch", Object: faceKey},
			minioSDK.CopySrcOptions{Bucket: "kwatch", Object: originKey},
		); err != nil {
			_ = config.S3Client.RemoveObject(ctx, "kwatch", originKey, minioSDK.RemoveObjectOptions{})
			sum.Skipped++
			sum.Errors = append(sum.Errors, kwatmod.SyncIbocError{ID: docID, Reason: "s3 copy face failed"})
			continue
		}

		// 4) เตรียม BulkWrite ($set)
		now := time.Now().UTC()
		set := bson.M{
			"photoOriginKey":         originKey,
			"photoOriginContentType": mime,
			"photoFaceKey":           faceKey,
			"photoFaceContentType":   mime,
			"photoKey":               originKey,
			"photoContentType":       mime,

			"external.iboc.id":       personID,
			"external.iboc.faceId":   faceID,
			"external.iboc.state":    "active",
			"external.iboc.syncedAt": now,

			"updatedAt": now,
		}
		writeModels = append(writeModels, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": oid}).
			SetUpdate(bson.M{"$set": set}),
		)

		// 5) per-item event (Kafka + MQTT)
		per := kwatmod.WatchlistEvent{
			ID:                     docID,
			Event:                  "watchlist.iboc.sync",
			TraceID:                traceID,
			Time:                   now.Format(time.RFC3339Nano),
			PhotoKey:               originKey,
			PhotoContentType:       mime,
			PhotoOriginKey:         originKey,
			PhotoOriginContentType: mime,
			PhotoFaceKey:           faceKey,
			External: map[string]any{
				"iboc": map[string]any{
					"id":       personID,
					"faceId":   faceID,
					"state":    "active",
					"syncedAt": now,
				},
			},
		}
		h := map[string]string{
			"event":   per.Event,
			"source":  "kwatsvc",
			"schema":  "kwatch/watchlistIbocSync/1",
			"traceId": traceID,
		}
		_ = kafka.PublishEventTo(ctx, "kwatch.watchlist", "iboc-sync-"+docID, per, h)
		_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.sync", map[string]any{
			"id":        per.ID,
			"event":     per.Event,
			"traceID":   per.TraceID,
			"state":     "active",
			"photoKey":  per.PhotoOriginKey,
			"faceKey":   per.PhotoFaceKey,
			"external":  per.External,
			"updatedAt": now,
		})

		if len(writeModels) >= bulkSize {
			flush()
		}
		sum.Updated++
	}

	// flush ค้าง
	flush()

	// summary (Kafka + MQTT)
	summaryPayload := map[string]any{
		"total":   sum.Total,
		"updated": sum.Updated,
		"skipped": sum.Skipped,
		"errors":  sum.Errors,
	}
	sEvt := kwatmod.WatchlistEvent{
		ID:      "summary",
		Event:   "watchlist.iboc.sync.summary",
		TraceID: traceID,
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		External: map[string]any{
			"summary": summaryPayload,
		},
	}
	hdr := map[string]string{
		"event":   sEvt.Event,
		"source":  "kwatsvc",
		"schema":  "kwatch/watchlistIbocSyncSummary/1",
		"traceId": traceID,
	}
	_ = kafka.PublishEventTo(ctx, "kwatch.watchlist", "iboc-sync-summary-"+traceID, sEvt, hdr)
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.sync.summary", map[string]any{
		"traceID": traceID,
		"total":   sum.Total,
		"updated": sum.Updated,
		"skipped": sum.Skipped,
		"errors":  sum.Errors,
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
	})

	return nil
}

// HandleSyncIbocDevTask: IBOC_DEV — ค้นหา face → ดาวน์โหลดรูป → อัปโหลด S3 → อัปเดต DB (bulk) → ยิง event
func HandleSyncIbocDevTask(parent context.Context, evt kwatmod.WatchlistEvent) error {
	ctx, end, log := traceutil.StartLite(parent, "klynx/kwatsvc", "kwatsvc.HandleSyncIbocDevTask", "kwatsvc", "HandleSyncIbocDevTask")
	defer end()

	traceID := strings.TrimSpace(evt.TraceID)
	if traceID == "" {
		traceID = traceutil.TraceIDFromCtx(ctx)
	}
	log = logger.FromCtx(ctx, "kwatsvc", "HandleSyncIbocDevTask").With().Str("traceID", traceID).Logger()

	// ---- parse payload ----
	task, _ := evt.External["task"].(map[string]any)
	itemsAny, _ := task["items"].([]any)
	if len(itemsAny) == 0 {
		log.Info().Msg("no items in ibocdev task, skip")
		return nil
	}

	// ---- gateway (IBOC_DEV) ----
	ibc := ibface.NewFromEnv("IBOC_DEV")
	if !ibc.Configured() {
		return fmt.Errorf("ibocdev not configured")
	}

	type result struct {
		wm    mongo.WriteModel
		event *kwatmod.WatchlistEvent
		err   *kwatmod.SyncIbocError
	}
	// worker pool
	const (
		workers  = 16
		bulkSize = 200
	)
	inCh := make(chan map[string]string, 1024)
	outCh := make(chan result, 1024)

	// producer
	go func() {
		for _, raw := range itemsAny {
			m, _ := raw.(map[string]any)
			id := strings.TrimSpace(fmt.Sprint(m["id"]))
			idc := strings.TrimSpace(fmt.Sprint(m["idcard"]))
			if id != "" && idc != "" {
				inCh <- map[string]string{"id": id, "idcard": idc}
			} else {
				// ส่ง error ออกทาง outCh เลย เพื่อให้รวมในสรุป
				outCh <- result{err: &kwatmod.SyncIbocError{ID: id, Reason: "empty id or idcard"}}
			}
		}
		close(inCh)
	}()

	// workers
	for w := 0; w < workers; w++ {
		go func() {
			for it := range inCh {
				docID := it["id"]
				idcard := it["idcard"]
				now := time.Now().UTC()

				oid, err := primitive.ObjectIDFromHex(docID)
				if err != nil {
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "bad hex _id"}}
					continue
				}

				// 1) query IBOC_DEV
				personID, faceID, imagePath, _, qErr := ibc.GetFaceByIdcard(ctx, idcard)
				if qErr != nil {
					// mark not_found — ลด requeue
					_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": oid}, bson.M{
						"external.ibocdev.state":    "not_found",
						"external.ibocdev.syncedAt": now,
						"updatedAt":                 now,
					})
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: qErr.Error()}}
					continue
				}

				// 1.5) ⬅️ NEW: update external.ibocdev ก่อนเลย
				_, _ = stomongo.UpdateOne(ctx, watchlistColl, bson.M{"_id": oid}, bson.M{
					"external.ibocdev.id":       personID,
					"external.ibocdev.faceId":   faceID,
					"external.ibocdev.state":    "active",
					"external.ibocdev.syncedAt": now,
					"updatedAt":                 now,
				})

				// 1.6) ⬅️ NEW: ผลัก metadata ล่าสุดกลับ IBOC_DEV ด้วย personID ที่มี (async)
				mctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := syncMetaBackToIBOC(mctx, ibc, personID, oid); err != nil {
					log.Warn().Err(err).Str("docID", docID).Str("personID", personID).Msg("⚠️ syncMetaBackToIBOC failed (prod)")
				} else {
					log.Info().Str("docID", docID).Str("personID", personID).Msg("✅ IBOC metadata updated (prod)")
				}
				cancel()

				// 2) ดาวน์โหลดรูปจาก storage (DEV)
				storageURL := faceStorageAbsoluteURLDev(ibc.BaseURL, imagePath)
				if storageURL == "" {
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "empty storage url"}}
					continue
				}
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, storageURL, nil)
				req.Header.Set("Accept", "image/*")
				resp, gErr := pooledHTTP.Do(req)
				if gErr != nil {
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "download image failed"}}
					continue
				}
				if resp == nil || resp.StatusCode >= 400 {
					if resp != nil {
						resp.Body.Close()
					}
					reason := "download image failed"
					if resp != nil {
						reason = fmt.Sprintf("http %d downloading image", resp.StatusCode)
					}
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: reason}}
					continue
				}
				imgBytes, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr != nil {
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "read image failed"}}
					continue
				}

				// 3) อัปโหลด S3: put origin → copy face
				mime := detectContentTypeLocal(imgBytes)
				ext := extForMimeLocal(mime)
				originKey := fmt.Sprintf("watchlist/%s/origin%s", docID, ext)
				faceKey := fmt.Sprintf("watchlist/%s/face%s", docID, ext)

				if _, err := config.S3Client.PutObject(ctx, "kwatch", originKey, bytes.NewReader(imgBytes), int64(len(imgBytes)), minioSDK.PutObjectOptions{ContentType: mime}); err != nil {
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "s3 upload origin failed"}}
					continue
				}
				if _, err := config.S3Client.CopyObject(ctx,
					minioSDK.CopyDestOptions{Bucket: "kwatch", Object: faceKey},
					minioSDK.CopySrcOptions{Bucket: "kwatch", Object: originKey},
				); err != nil {
					_ = config.S3Client.RemoveObject(ctx, "kwatch", originKey, minioSDK.RemoveObjectOptions{})
					outCh <- result{err: &kwatmod.SyncIbocError{ID: docID, Reason: "s3 copy face failed"}}
					continue
				}

				// 4) เตรียม bulk update + per-item event
				set := bson.M{
					"photoOriginKey":         originKey,
					"photoOriginContentType": mime,
					"photoFaceKey":           faceKey,
					"photoFaceContentType":   mime,
					"photoKey":               originKey,
					"photoContentType":       mime,

					"external.ibocdev.id":       personID,
					"external.ibocdev.faceId":   faceID,
					"external.ibocdev.state":    "active",
					"external.ibocdev.syncedAt": now,

					"updatedAt": now,
				}

				wm := mongo.NewUpdateOneModel().
					SetFilter(bson.M{"_id": oid}).
					SetUpdate(bson.M{"$set": set})

				per := kwatmod.WatchlistEvent{
					ID:                     docID,
					Event:                  "watchlist.ibocdev.sync",
					TraceID:                traceID,
					Time:                   now.Format(time.RFC3339Nano),
					PhotoKey:               originKey,
					PhotoContentType:       mime,
					PhotoOriginKey:         originKey,
					PhotoOriginContentType: mime,
					PhotoFaceKey:           faceKey,
					External: map[string]any{
						"ibocdev": map[string]any{
							"id":       personID,
							"faceId":   faceID,
							"state":    "active",
							"syncedAt": now,
						},
					},
				}
				outCh <- result{wm: wm, event: &per}
			}
		}()
	}

	// collector: bulk write + fire events
	sumTotal := 0
	sumUpdated := 0
	var errs []kwatmod.SyncIbocError
	writeModels := make([]mongo.WriteModel, 0, bulkSize)

	flush := func() {
		if len(writeModels) == 0 {
			return
		}
		_, _ = stomongo.BulkWrite(ctx, watchlistColl, writeModels, options.BulkWrite().SetOrdered(false))
		writeModels = writeModels[:0]
	}

	done := make(chan struct{})
	go func() {
		processed := 0
		for processed < len(itemsAny) {
			r := <-outCh
			processed++
			sumTotal++

			if r.err != nil {
				errs = append(errs, *r.err)
				continue
			}
			// enqueue write model
			if r.wm != nil {
				writeModels = append(writeModels, r.wm)
				sumUpdated++
				if len(writeModels) >= bulkSize {
					flush()
				}
			}
			// fire per-item event
			if r.event != nil {
				_ = kafka.PublishEventTo(ctx, "kwatch.watchlist", "ibocdev-sync-"+r.event.ID, *r.event, map[string]string{
					"event":   r.event.Event,
					"source":  "kwatsvc",
					"schema":  "kwatch/watchlistIbocDevSync/1",
					"traceId": traceID,
				})
				// FE MQTT (optional)
				_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.ibocdev.sync", map[string]any{
					"id":        r.event.ID,
					"event":     r.event.Event,
					"traceID":   r.event.TraceID,
					"state":     "active",
					"photoKey":  r.event.PhotoOriginKey,
					"faceKey":   r.event.PhotoFaceKey,
					"external":  r.event.External,
					"updatedAt": r.event.Time,
				})
			}
		}
		close(done)
	}()

	<-done
	flush()

	// summary event
	summaryPayload := map[string]any{
		"total":   sumTotal,
		"updated": sumUpdated,
		"skipped": sumTotal - sumUpdated,
		"errors":  errs,
	}
	sEvt := kwatmod.WatchlistEvent{
		ID:      "summary",
		Event:   "watchlist.ibocdev.sync.summary",
		TraceID: traceID,
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		External: map[string]any{
			"summary": summaryPayload,
		},
	}
	_ = kafka.PublishEventTo(ctx, "kwatch.watchlist", "ibocdev-sync-summary-"+traceID, sEvt, map[string]string{
		"event":   sEvt.Event,
		"source":  "kwatsvc",
		"schema":  "kwatch/watchlistIbocDevSyncSummary/1",
		"traceId": traceID,
	})

	return nil
}

// ---------- small helpers used above ----------

func detectContentTypeLocal(b []byte) string {
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg"
	}
	return "image/jpeg"
}

func extForMimeLocal(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	default:
		return ".jpg"
	}
}

// สร้าง URL storage จาก imagePath โดยใช้ ENV ชัดเจน
// Priority: IBOC_6_STORAGE -> IBOC_STORAGE -> เดา api.* → storage.*
func faceStorageAbsoluteURL(baseAPI, imagePath string) string {
	imagePath = strings.TrimLeft(strings.TrimSpace(imagePath), "/")
	if imagePath == "" {
		return ""
	}
	if s := strings.TrimSpace(os.Getenv("IBOC_6_STORAGE")); s != "" {
		return strings.TrimRight(s, "/") + "/" + imagePath
	}
	if s := strings.TrimSpace(os.Getenv("IBOC_STORAGE")); s != "" {
		return strings.TrimRight(s, "/") + "/" + imagePath
	}
	// fallback: เดาเอาจาก baseAPI
	u, err := url.Parse(strings.TrimSpace(baseAPI))
	if err != nil || u.Host == "" {
		return strings.TrimRight(baseAPI, "/") + "/" + imagePath
	}
	host := u.Host
	if strings.HasPrefix(host, "api.") {
		host = "storage." + strings.TrimPrefix(host, "api.")
	}
	u.Host = host
	u.Path = path.Join("/", imagePath)
	return u.String()
}

// DEV: Priority: IBOC_DEV_STORAGE -> IBOC_STORAGE -> เดา api.* → storage.*
func faceStorageAbsoluteURLDev(baseAPI, imagePath string) string {
	imagePath = strings.TrimLeft(strings.TrimSpace(imagePath), "/")
	if imagePath == "" {
		return ""
	}
	if s := strings.TrimSpace(os.Getenv("IBOC_DEV_STORAGE")); s != "" {
		return strings.TrimRight(s, "/") + "/" + imagePath
	}
	if s := strings.TrimSpace(os.Getenv("IBOC_STORAGE")); s != "" {
		return strings.TrimRight(s, "/") + "/" + imagePath
	}
	// fallback: เดา
	u, err := url.Parse(strings.TrimSpace(baseAPI))
	if err != nil || u.Host == "" {
		return strings.TrimRight(baseAPI, "/") + "/" + imagePath
	}
	host := u.Host
	if strings.HasPrefix(host, "api.") {
		host = "storage." + strings.TrimPrefix(host, "api.")
	}
	u.Host = host
	u.Path = path.Join("/", imagePath)
	return u.String()
}

// ===== Helpers: push latest metadata back to IBOC using known personID =====

// ===== Helpers: push latest metadata back to IBOC using known personID =====

// ===== Helpers: push latest metadata back to IBOC using known personID =====

func syncMetaBackToIBOC(ctx context.Context, cli *ibface.Client, knownPersonID string, objID primitive.ObjectID) error {
	if cli == nil || !cli.Configured() {
		return fmt.Errorf("iboc client not configured")
	}
	if objID == primitive.NilObjectID {
		return fmt.Errorf("nil object id")
	}

	// 1) โหลดข้อมูลล่าสุดจาก DB
	cur, err := loadWatchlistCore(ctx, objID)
	if err != nil || cur == nil {
		return fmt.Errorf("loadWatchlistCore failed: %w", err)
	}

	// 2) เตรียมค่าที่ IBOC ต้องการ (ใช้กติกาเดียวกับของเดิม)
	alertTitle, crimesTitle, stationTitle := computeTitlesForIBOC(ctx, cur, objID)

	// 3) ชื่อ-นามสกุล: กันเคสว่าง (สาเหตุ 400)
	firstName, lastName := deriveNamesForIBOC(ctx, objID, cur)
	if strings.TrimSpace(firstName) == "" && strings.TrimSpace(lastName) == "" {
		return fmt.Errorf("empty first/last name after fallback")
	}

	// 4) Ensure (ถ้าจำเป็น) แล้ว UpdatePerson โดยตรง (enabled=true ภายใน UpdatePerson)
	pid := strings.TrimSpace(knownPersonID)
	if pid == "" {
		pp, e := cli.EnsurePerson(ctx,
			firstName, lastName, cur.IdCard,
			alertTitle, cur.AlertDesc, crimesTitle, stationTitle)
		if e != nil || strings.TrimSpace(pp) == "" {
			return fmt.Errorf("ensurePerson failed: %w", e)
		}
		pid = pp
	}

	if err := cli.UpdatePerson(ctx, pid,
		firstName, lastName, cur.IdCard,
		alertTitle, cur.AlertDesc, crimesTitle, stationTitle); err != nil {
		// edge: ถ้า update ล้มเหลว (เช่น person โดนลบ) → ensure แล้ว update ซ้ำ
		pp, e := cli.EnsurePerson(ctx,
			firstName, lastName, cur.IdCard,
			alertTitle, cur.AlertDesc, crimesTitle, stationTitle)
		if e != nil || strings.TrimSpace(pp) == "" {
			return fmt.Errorf("updatePerson failed and ensure failed: %w", err)
		}
		if ue := cli.UpdatePerson(ctx, pp,
			firstName, lastName, cur.IdCard,
			alertTitle, cur.AlertDesc, crimesTitle, stationTitle); ue != nil {
			return fmt.Errorf("updatePerson retry failed: %w", ue)
		}
		pid = pp
	}

	// 5) upsert external → active + syncedAt now (faceId ไม่แตะ)
	_ = upsertExternal(ctx, namespaceFromBaseURL(cli.BaseURL), objID, pid, nil, "active")
	return nil
}

// === ชื่อ-นามสกุล fallback กัน 400 จาก IBOC ===
// === ชื่อ-นามสกุล fallback กัน 400 จาก IBOC (รองรับต่างชาติ) ===
// === ชื่อ-นามสกุล fallback กัน 400 จาก IBOC (รองรับต่างชาติ, ไม่อ้าง field ที่ struct ไม่มี) ===
func deriveNamesForIBOC(ctx context.Context, id primitive.ObjectID, cur *kwatmod.WatchlistCoreForIBOC) (string, string) {
	// 1) ใช้ค่าจาก struct ก่อน (TH)
	first := strings.TrimSpace(cur.FirstName)
	last := strings.TrimSpace(cur.LastName)
	if first != "" || last != "" {
		return sanitizeName(first), sanitizeName(last)
	}

	// 2) ดึงทั้ง TH/EN จาก DB (projection)
	var aux struct {
		FirstTH string `bson:"firstname"`
		LastTH  string `bson:"lastname"`
		FirstEN string `bson:"firstNameEn"`
		LastEN  string `bson:"lastNameEn"`
		FullEN  string `bson:"fullNameEn"`
		FullTH  string `bson:"fullName"`
		TitleTH string `bson:"titlename"`
	}
	_ = config.DB.Collection(watchlistColl).
		FindOne(ctx, bson.M{"_id": id},
			options.FindOne().SetProjection(bson.M{
				"firstname":   1,
				"lastname":    1,
				"firstNameEn": 1,
				"lastNameEn":  1,
				"fullName":    1,
				"fullNameEn":  1,
				"titlename":   1,
			}),
		).Decode(&aux)

	// 3) ถ้ามี EN ใน DB ให้ใช้ก่อน (กรณีต่างชาติ)
	if strings.TrimSpace(aux.FirstEN) != "" || strings.TrimSpace(aux.LastEN) != "" {
		if first == "" {
			first = strings.TrimSpace(aux.FirstEN)
		}
		if last == "" {
			last = strings.TrimSpace(aux.LastEN)
		}
		return sanitizeName(first), sanitizeName(last)
	}

	// 4) ถัดมาใช้ TH ใน DB
	if strings.TrimSpace(aux.FirstTH) != "" || strings.TrimSpace(aux.LastTH) != "" {
		if first == "" {
			first = strings.TrimSpace(aux.FirstTH)
		}
		if last == "" {
			last = strings.TrimSpace(aux.LastTH)
		}
		return sanitizeName(first), sanitizeName(last)
	}

	// 5) แตกจาก fullNameEn ก่อน แล้วค่อย fullName (ตัดคำนำหน้าไทย)
	if s := strings.TrimSpace(aux.FullEN); s != "" {
		fn, ln := splitFullName(s, true)
		if fn != "" || ln != "" {
			return sanitizeName(fn), sanitizeName(ln)
		}
	}
	if s := strings.TrimSpace(aux.FullTH); s != "" {
		fn, ln := splitFullName(removeThaiTitle(s), false)
		if fn != "" || ln != "" {
			return sanitizeName(fn), sanitizeName(ln)
		}
	}

	// 6) ไม่พบจริง ๆ → ปล่อยว่างให้ caller ตัดสินใจ (จะได้ error ชัดว่า empty name)
	return "", ""
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	// รวมช่องว่างหลายตัวให้เป็นตัวเดียว
	return strings.Join(strings.Fields(s), " ")
}

// แยก full name → first/last (ภาษาอังกฤษ: ตัดคำนำหน้าทั่วไป, ภาษาไทย: ทำไปแล้วด้านบน)
func splitFullName(full string, isEN bool) (first, last string) {
	name := strings.TrimSpace(full)
	if isEN {
		name = removeEnglishTitle(name)
	}
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
	}
}

func removeThaiTitle(s string) string {
	t := strings.TrimSpace(s)
	prefixes := []string{
		"นาย", "นางสาว", "น.ส.", "นาง",
		"ด.ช.", "ด.ญ.",
		"ว่าที่ ร.ต.", "ว่าที่ ร.ท.", "ว่าที่ ร.อ.",
		"ร.ต.", "ร.ท.", "ร.อ.",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p+" ") {
			return strings.TrimSpace(strings.TrimPrefix(t, p))
		}
	}
	return t
}

func removeEnglishTitle(s string) string {
	t := strings.TrimSpace(s)
	prefixes := []string{
		"Mr.", "Mrs.", "Ms.", "Miss", "Mx.",
		"Dr.", "Prof.", "Sir", "Madam", "Dame",
		"Khun", "Khun.", // บางครั้งใช้ทับศัพท์
	}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p+" ") {
			return strings.TrimSpace(strings.TrimPrefix(t, p))
		}
	}
	return t
}

// แปลง BaseURL → "iboc" หรือ "ibocdev" เพื่อใช้กับ upsertExternal
func namespaceFromBaseURL(base string) string {
	b := strings.ToLower(strings.TrimSpace(base))
	switch {
	case strings.Contains(b, "-poc.") || strings.Contains(b, "dev"):
		return "ibocdev"
	default:
		return "iboc"
	}
}

// ===== Helpers to read latest metadata from DB & compute titles =====

// โหลดข้อมูล watchlist ล่าสุดจาก DB (ใช้ struct จาก kwatmod)
func loadWatchlistCore(ctx context.Context, id primitive.ObjectID) (*kwatmod.WatchlistCoreForIBOC, error) {
	if id == primitive.NilObjectID {
		return nil, fmt.Errorf("nil object id")
	}
	var cur kwatmod.WatchlistCoreForIBOC
	if err := config.DB.Collection(watchlistColl).FindOne(ctx, bson.M{"_id": id}).Decode(&cur); err != nil {
		return nil, err
	}
	return &cur, nil
}

// fallback หา station title จาก warrants หากไม่มี code
func getFirstWarrantStationTitleFromDB(ctx context.Context, id primitive.ObjectID) string {
	if id == primitive.NilObjectID {
		return ""
	}
	var cur struct {
		Warrants []struct {
			PoliceStation string `bson:"policeStation"`
		} `bson:"warrants"`
	}
	_ = config.DB.Collection(watchlistColl).FindOne(ctx, bson.M{"_id": id}).Decode(&cur)
	if len(cur.Warrants) > 0 {
		return strings.TrimSpace(cur.Warrants[0].PoliceStation)
	}
	return ""
}

// คำนวณ alertTitle / crimesTitle / stationTitle ด้วยกติกาเดียวกับ HandleWatchlistUpdate
// หมายเหตุ: resolveAlertTitle/resolveCrimesTitle/resolvePoliceStationTitle ต้องมีใน package นี้
func computeTitlesForIBOC(ctx context.Context, cur *kwatmod.WatchlistCoreForIBOC, objID primitive.ObjectID) (alertTitle, crimesTitle, stationTitle string) {
	alertTitle, _ = resolveAlertTitle(ctx, strings.TrimSpace(cur.AlertType))
	crimesTitle, _ = resolveCrimesTitle(ctx, cur.CrimesType)

	if cur.PoliceStation > 0 {
		st, _ := resolvePoliceStationTitle(ctx, cur.PoliceProvincial, cur.PoliceStation)
		stationTitle = st
	} else {
		st := strings.TrimSpace(cur.StationTitleFallback)
		if st == "" {
			st = getFirstWarrantStationTitleFromDB(ctx, objID)
		}
		stationTitle = st
	}
	return
}
