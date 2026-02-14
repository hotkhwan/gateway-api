// internal/services/kwatsvc/handleWatchlistDelete.go
package kwatsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/mqtt/kwatchmsg"
	"klynx/internal/repo/stomongo"
	"klynx/internal/repo/stos3minio"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	"github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ปรับได้ตาม workload ของคุณ
const (
	deleteOverallTimeout = 25 * time.Second
	watchmanTimeout      = 5 * time.Second
	s3TimeoutPerKey      = 10 * time.Second
	mongoTimeout         = 5 * time.Second
	mqttTimeout          = 2 * time.Second
)

// เปิด strict mode เพื่อให้ return error แล้ว Kafka retry (ระวัง lag โตถ้า downstream ช้า/ค้าง)
func isDeleteStrict() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("KWATCH_DELETE_STRICT")))
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

type stepError struct {
	step string
	err  error
}

func (e stepError) Error() string {
	if e.err == nil {
		return e.step + ": <nil>"
	}
	return fmt.Sprintf("%s: %v", e.step, e.err)
}

// runBounded ทำให้ "งานเสี่ยงค้าง" ไม่สามารถบล็อก handler ได้
// หมายเหตุ: ถ้า fn ไม่ respect ctx อาจเกิด goroutine leak ได้ แต่ handler จะไม่ค้างแล้ว (แก้ lag โตทันที)
func runBounded(parent context.Context, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// ป้องกัน panic ทำให้ handler ค้าง
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func HandleWatchlistDelete(parent context.Context, evt kwatmod.WatchlistEvent, g Gateways) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"klynx/kwatsvc",
		"watchlist.HandleWatchlistDelete",
		"kwatsvc", "HandleWatchlistDelete",
	)
	defer end()

	// overall hard limit
	ctx, cancel := context.WithTimeout(ctx, deleteOverallTimeout)
	defer cancel()

	// เติม external จาก top-level ถ้ายังไม่มี
	if len(evt.External) == 0 {
		evt.External = map[string]any{}
		if evt.IBOCTop.ID != "" || evt.IBOCTop.FaceID != "" {
			evt.External["iboc"] = map[string]any{"id": evt.IBOCTop.ID, "faceId": evt.IBOCTop.FaceID}
		}
		if evt.IBOCDevTop.ID != "" || evt.IBOCDevTop.FaceID != "" {
			evt.External["ibocdev"] = map[string]any{"id": evt.IBOCDevTop.ID, "faceId": evt.IBOCDevTop.FaceID}
		}
		if evt.WatchmanTop.ID != "" || evt.WatchmanTop.IDCard != "" {
			evt.External["watchman"] = map[string]any{"id": evt.WatchmanTop.ID, "idCard": evt.WatchmanTop.IDCard}
		}
	}

	// hydrate เมื่อจำเป็นจริง ๆ (เดิมของคุณทำเฉพาะ crimes.delete)
	need := needHydrate(evt) && strings.EqualFold(evt.Event, "watchlist.crimes.delete")
	if need {
		if _, err := hydrateDeleteContext(ctx, &evt); err != nil {
			if hasDeleteKeys(evt) {
				log.Warn().Err(err).Str("id", evt.ID).Msg("hydrate failed but proceed with existing keys")
			} else {
				log.Error().Err(err).Str("id", evt.ID).Msg("hydrate failed and no keys to delete")
			}
		}
	}

	// ----- resolve keys/ids สำหรับลบ third-party -----
	ibocFaceID, ibocPersonID := extractIDsFromEvtExternal(evt.External, "iboc")
	ibocDevFaceID, ibocDevPersonID := extractIDsFromEvtExternal(evt.External, "ibocdev")

	watchmanIDCard := extractWatchmanIDCardFromEvtExternal(evt.External)
	if watchmanIDCard == "" {
		// fallback chain
		watchmanIDCard = firstNonEmpty(firstNonEmpty(evt.IdCard, evt.PersonKey), asIDCardIfNumeric(evt.ID))
	}

	// ----- purge S3 keys -----
	keys := make(map[string]struct{})
	addKey := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			keys[s] = struct{}{}
		}
	}
	addKey(evt.PhotoKey)       // compat
	addKey(evt.PhotoOriginKey) // origin
	addKey(evt.PhotoFaceKey)   // face

	// ----- DB filter -----
	var filter bson.M
	switch {
	case func() bool {
		_, err := primitive.ObjectIDFromHex(evt.ID)
		return err == nil
	}():
		oid, _ := primitive.ObjectIDFromHex(evt.ID)
		filter = bson.M{"_id": oid}
	case strings.TrimSpace(evt.PersonKey) != "":
		filter = bson.M{"personKey": strings.TrimSpace(evt.PersonKey), "source": "crimes"}
	case strings.TrimSpace(evt.IdCard) != "":
		filter = bson.M{"idcard": strings.TrimSpace(evt.IdCard), "source": "crimes"}
	default:
		filter = nil
	}

	var stepErrs []error

	// ----- IBOC deletes (prod/dev) -----
	if g.IBOCProd != nil && (ibocFaceID != "" || ibocPersonID != "") {
		err := runBounded(ctx, 6*time.Second, func(c context.Context) error {
			return g.IBOCProd.DeletePerson(c, ibocPersonID, ibocFaceID)
		})
		if err != nil {
			e := stepError{step: "ibocProdDelete", err: err}
			stepErrs = append(stepErrs, e)
			log.Error().Err(err).Str("personId", ibocPersonID).Str("faceId", ibocFaceID).Msg("IBOC prod delete error")
		} else {
			log.Info().Str("personId", ibocPersonID).Str("faceId", ibocFaceID).Msg("✅ IBOC prod deleted")
		}
	}

	if g.IBOCDev != nil && (ibocDevFaceID != "" || ibocDevPersonID != "") {
		err := runBounded(ctx, 6*time.Second, func(c context.Context) error {
			return g.IBOCDev.DeletePerson(c, ibocDevPersonID, ibocDevFaceID)
		})
		if err != nil {
			e := stepError{step: "ibocDevDelete", err: err}
			stepErrs = append(stepErrs, e)
			log.Error().Err(err).Str("personId", ibocDevPersonID).Str("faceId", ibocDevFaceID).Msg("IBOC dev delete error")
		} else {
			log.Info().Str("personId", ibocDevPersonID).Str("faceId", ibocDevFaceID).Msg("✅ IBOC dev deleted")
		}
	}

	// ----- Watchman delete (เสี่ยงค้างที่สุด) -----
	if g.Watchman != nil && strings.TrimSpace(watchmanIDCard) != "" {
		err := runBounded(ctx, watchmanTimeout, func(c context.Context) error {
			st, body, e := g.Watchman.DeleteByID(c, watchmanIDCard)
			if e != nil {
				return fmt.Errorf("watchman http err: %w", e)
			}
			if st >= 400 {
				return fmt.Errorf("watchman status=%d resp=%s", st, string(body))
			}
			return nil
		})

		if err != nil {
			e := stepError{step: "watchmanDelete", err: err}
			stepErrs = append(stepErrs, e)
			log.Error().Err(err).Str("idcard", watchmanIDCard).Msg("⚠️ Watchman delete failed")
		} else {
			log.Debug().Str("idcard", watchmanIDCard).Msg("✅ Watchman deleted by idcard")
		}
	}

	// ----- S3 delete (ทำทีละ key เพื่อควบคุม timeout) -----
	if len(keys) == 0 {
		log.Debug().Msg("🧹 no photo keys to purge")
	}
	for k := range keys {
		key := k
		err := runBounded(ctx, s3TimeoutPerKey, func(c context.Context) error {
			return stos3minio.DeleteFile(c, key)
		})
		if err != nil {
			e := stepError{step: "s3Delete:" + key, err: err}
			stepErrs = append(stepErrs, e)

			var er minio.ErrorResponse
			if errors.As(err, &er) {
				log.Error().
					Str("object", key).
					Int("status", er.StatusCode).
					Str("code", er.Code).
					Str("requestID", er.RequestID).
					Str("hostID", er.HostID).
					Msg("❌ S3 delete failed")
			} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				log.Error().Str("object", key).Err(err).Msg("❌ S3 delete timeout/canceled")
			} else {
				log.Error().Str("object", key).Err(err).Msg("❌ S3 delete failed (unknown)")
			}
		} else {
			log.Debug().Str("object", key).Msg("🗑 S3 object deleted")
		}
	}

	// ----- mongo update (อย่าทิ้ง error) -----
	if filter != nil {
		now := time.Now().UTC()
		set, unset := buildExternalDeleteOps(g, now)

		err := runBounded(ctx, mongoTimeout, func(c context.Context) error {
			_, e := stomongo.UpdateOneOps(c, watchlistColl, filter, set, unset)
			return e
		})
		if err != nil {
			e := stepError{step: "mongoUpdate", err: err}
			stepErrs = append(stepErrs, e)
			log.Error().Err(err).Interface("filter", filter).Msg("❌ mongo update failed")
		}
	}

	// ----- publish mqtt (อย่าบล็อก) -----
	_ = runBounded(ctx, mqttTimeout, func(c context.Context) error {
		// NOTE: function นี้ไม่รับ ctx แต่เราบังคับไม่ให้ handler รอเกิน mqttTimeout ด้วย runBounded
		return kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.deleted", "msg data delete is done")
	})

	// ---- decision: strict หรือ commit-forward ----
	if len(stepErrs) > 0 {
		// ส่งสัญญาณให้รู้ว่ามีบาง step fail (เพื่อ debug)
		log.Warn().
			Str("event", strings.TrimSpace(evt.Event)).
			Str("id", strings.TrimSpace(evt.ID)).
			Int("failedSteps", len(stepErrs)).
			Msg("⚠️ delete completed with failures")

		// ถ้า strict: ให้ Kafka retry (แต่จะทำให้ lag โต ถ้า downstream ค้าง)
		if isDeleteStrict() {
			return errors.Join(stepErrs...)
		}

		// default: commit-forward เพื่อไม่ให้ lag โต
		return nil
	}

	return nil
}

// ------- helpers: อ่านจาก evt.external -------

// helper: สร้าง set/unset ตาม gateways ที่เปิดจริง
func buildExternalDeleteOps(g Gateways, now time.Time) (bson.M, bson.M) {
	set := bson.M{
		"state":            "archived",
		"updatedAt":        now,
		"deletedAt":        now,
		"isDeleted":        true,
		"photoKey":         "",
		"photoContentType": "",
	}
	unset := bson.M{
		"photoOriginKey":         1,
		"photoOriginContentType": 1,
		"photoFaceKey":           1,
		"photoFaceContentType":   1,
	}

	for _, ns := range g.ConfiguredNamespaces() {
		set["external."+ns+".state"] = "deleted"
		set["external."+ns+".syncedAt"] = now
		switch ns {
		case "iboc", "ibocdev":
			unset["external."+ns+".id"] = 1
			unset["external."+ns+".faceId"] = 1
		case "watchman":
			unset["external."+ns+".id"] = 1
		}
	}
	return set, unset
}

func extractIDsFromEvtExternal(external map[string]any, ns string) (faceID, personID string) {
	if external == nil {
		return "", ""
	}
	m, _ := external[ns].(map[string]any)
	if m == nil {
		return "", ""
	}
	if v, ok := m["faceId"].(string); ok {
		faceID = v
	}
	if v, ok := m["id"].(string); ok {
		personID = v
	}
	return
}

func extractWatchmanIDCardFromEvtExternal(external map[string]any) string {
	if external == nil {
		return ""
	}
	m, _ := external["watchman"].(map[string]any)
	if m == nil {
		return ""
	}
	if v, ok := m["idcard"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	if v, ok := m["idCard"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return ""
}

func hydrateDeleteContext(ctx context.Context, evt *kwatmod.WatchlistEvent) (*kwatmod.WlMini, error) {
	var doc kwatmod.WlMini
	filter := bson.M{"source": "crimes"}

	switch {
	case func() bool {
		_, err := primitive.ObjectIDFromHex(evt.ID)
		return err == nil
	}():
		oid, _ := primitive.ObjectIDFromHex(evt.ID)
		filter["_id"] = oid
	case strings.TrimSpace(evt.PersonKey) != "":
		filter["personKey"] = strings.TrimSpace(evt.PersonKey)
	case strings.TrimSpace(evt.IdCard) != "":
		filter["idcard"] = strings.TrimSpace(evt.IdCard)
	default:
		return nil, fmt.Errorf("no key to resolve (need _id/personKey/idcard)")
	}

	prj := bson.M{
		"personKey": 1, "idcard": 1,
		"photoKey": 1, "photoOriginKey": 1, "photoFaceKey": 1,
		"external": 1,
	}

	coll := config.DB.Collection("kwatch_watchlist")
	if err := coll.FindOne(ctx, filter, options.FindOne().SetProjection(prj)).Decode(&doc); err != nil {
		return nil, err
	}

	// เติมกลับเข้า evt ถ้าขาด
	if strings.TrimSpace(evt.PhotoKey) == "" {
		evt.PhotoKey = doc.PhotoKey
	}
	if strings.TrimSpace(evt.PhotoOriginKey) == "" {
		evt.PhotoOriginKey = doc.PhotoOriginKey
	}
	if strings.TrimSpace(evt.PhotoFaceKey) == "" {
		evt.PhotoFaceKey = doc.PhotoFaceKey
	}
	if strings.TrimSpace(evt.PersonKey) == "" {
		evt.PersonKey = doc.PersonKey
	}
	if strings.TrimSpace(evt.IdCard) == "" {
		evt.IdCard = doc.IDCard
	}

	if evt.External == nil {
		evt.External = map[string]any{}
	}
	if _, ok := evt.External["iboc"]; !ok {
		evt.External["iboc"] = map[string]any{"id": doc.External.IBOC.ID, "faceId": doc.External.IBOC.FaceID}
	}
	if _, ok := evt.External["ibocdev"]; !ok && (doc.External.IBOCDev.ID != "" || doc.External.IBOCDev.FaceID != "") {
		evt.External["ibocdev"] = map[string]any{"id": doc.External.IBOCDev.ID, "faceId": doc.External.IBOCDev.FaceID}
	}
	if _, ok := evt.External["watchman"]; !ok && (doc.External.Watchman.ID != "" || doc.External.Watchman.IDCard != "") {
		evt.External["watchman"] = map[string]any{"id": doc.External.Watchman.ID, "idCard": doc.External.Watchman.IDCard}
	}
	return &doc, nil
}

// ---- small helpers ----
func needHydrate(evt kwatmod.WatchlistEvent) bool {
	empty := func(s string) bool { return strings.TrimSpace(s) == "" }
	return (len(evt.External) == 0) ||
		(empty(evt.PhotoKey) && empty(evt.PhotoOriginKey) && empty(evt.PhotoFaceKey))
}

func hasDeleteKeys(e kwatmod.WatchlistEvent) bool {
	if strings.TrimSpace(e.IdCard) != "" {
		return true
	}
	if _, pid := extractIDsFromEvtExternal(e.External, "iboc"); pid != "" {
		return true
	}
	if _, pid := extractIDsFromEvtExternal(e.External, "ibocdev"); pid != "" {
		return true
	}
	if extractWatchmanIDCardFromEvtExternal(e.External) != "" {
		return true
	}
	return false
}

func asIDCardIfNumeric(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}
