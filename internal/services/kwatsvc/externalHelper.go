// internal/services/kwatsvc/externalHelper.go
package kwatsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const watchlistColl = "kwatch_watchlist"

// SetExternalNS
// - เขียน external.<ns> = { id, syncedAt, state[, extra] } และอัพเดต updatedAt
// - overwrite=false: เขียนเฉพาะกรณีที่ยังไม่มี external.<ns>.id
// - overwrite=true: บังคับเขียนทับ
func SetExternalNS(
	ctx context.Context,
	coll string,
	docID primitive.ObjectID,
	ns string,
	ref kwatmod.ExternalRef,
	overwrite bool,
) (matched, modified int64, err error) {

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now().UTC()
	if ref.SyncedAt.IsZero() {
		ref.SyncedAt = now
	}
	if ref.State == "" {
		ref.State = "active" // default
	}

	nsPrefix := fmt.Sprintf("external.%s", ns)
	set := bson.M{
		nsPrefix + ".id":       ref.ID,
		nsPrefix + ".syncedAt": ref.SyncedAt,
		nsPrefix + ".state":    ref.State,
		"updatedAt":            now,
	}

	if overwrite {
		// บังคับเขียนทับ
		res, e := stomongo.UpdateByID(opCtx, coll, docID, set)
		if e != nil {
			return 0, 0, e
		}
		return res.MatchedCount, res.ModifiedCount, nil
	}

	// เขียนเฉพาะกรณีที่ยังไม่มี id
	filter := bson.M{
		"_id": docID,
		"$or": []bson.M{
			{nsPrefix: bson.M{"$exists": false}},
			{nsPrefix + ".id": bson.M{"$exists": false}},
			{nsPrefix + ".id": 0},  // กันค่าศูนย์ (ถ้าเป็นเลข)
			{nsPrefix + ".id": ""}, // กันค่าว่าง (ถ้าเป็นสตริง)
		},
	}
	res, e := stomongo.UpdateOne(opCtx, coll, filter, set)
	if e != nil {
		return 0, 0, e
	}
	return res.MatchedCount, res.ModifiedCount, nil
}

// MarkExternalState
// - เปลี่ยนเฉพาะ state + syncedAt (ต้องมี external.<ns>.id แล้ว)
func MarkExternalState(
	ctx context.Context,
	coll string,
	docID primitive.ObjectID,
	ns string,
	state string, // "active" | "updated" | "deleted" | "error"
) error {
	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now().UTC()
	nsPrefix := fmt.Sprintf("external.%s", ns)

	_, err := stomongo.UpdateOne(opCtx, coll,
		bson.M{
			"_id":            docID,
			nsPrefix + ".id": bson.M{"$exists": true},
		},
		bson.M{
			"$set": bson.M{
				nsPrefix + ".state":    state,
				nsPrefix + ".syncedAt": now,
				"updatedAt":            now,
			},
		},
	)
	return err
}

// ใช้ $set เสมอ ป้องกัน replace documentทั้งก้อน + รองรับลบ faceId เมื่อส่ง pointer &"" เข้ามา
func upsertExternal(
	ctx context.Context,
	ns string,
	docID primitive.ObjectID,
	personID string,
	faceID *string, // nil = ไม่แตะ, &"" = unset, &"xxx" = set
	state string,
) error {
	if state == "" {
		state = "active"
	}
	now := time.Now().UTC()

	set := bson.M{
		fmt.Sprintf("external.%s.id", ns):       personID,
		fmt.Sprintf("external.%s.state", ns):    state,
		fmt.Sprintf("external.%s.syncedAt", ns): now,
	}
	unset := bson.M{}

	if faceID != nil {
		key := fmt.Sprintf("external.%s.faceId", ns)
		if *faceID == "" {
			unset[key] = 1
		} else {
			set[key] = *faceID
		}
	}

	return httputil.Retry(ctx,
		httputil.RetryCfg{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second, Jitter: true},
		func(_ int) (bool, error) {
			var err error
			if len(unset) > 0 {
				_, err = stomongo.UpdateByIDOps(ctx, watchlistColl, docID, set, unset) // $set + $unset (+ updatedAt อัตโนมัติ)
			} else {
				_, err = stomongo.UpdateByID(ctx, watchlistColl, docID, set) // $set (+ updatedAt อัตโนมัติ)
			}
			return httputil.RetryableErr(err), err
		},
	)
}

func resolveAlertTitle(ctx context.Context, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", nil
	}
	var v struct {
		AlertType []struct {
			ID    string `bson:"id" json:"id"`
			Title string `bson:"title" json:"title"`
		} `bson:"alertType" json:"alertType"`
	}

	// ✅ FindOne(ctx, coll, filter, outPtr) คืน error อย่างเดียว
	if err := stomongo.FindOne(ctx, "options", bson.M{"_id": "list.kwatch"}, &v); err != nil {
		return id, err // หาไม่ได้ → ใช้ id คืนไป
	}
	for _, it := range v.AlertType {
		if it.ID == id && strings.TrimSpace(it.Title) != "" {
			return it.Title, nil
		}
	}
	return id, nil
}

func resolveCrimesTitle(ctx context.Context, id int) (string, error) {
	if id == 0 {
		return "", nil
	}

	var v struct {
		CrimesType []struct {
			ID    any    `bson:"id" json:"id"`
			Title string `bson:"title" json:"title"`
		} `bson:"crimesType" json:"crimesType"`
	}
	if err := stomongo.FindOne(ctx, "options", bson.M{"_id": "list.kwatch"}, &v); err != nil {
		return strconv.Itoa(id), err
	}
	idStr := strconv.Itoa(id)
	for _, it := range v.CrimesType {
		switch x := it.ID.(type) {
		case string:
			if strings.TrimSpace(x) == idStr && strings.TrimSpace(it.Title) != "" {
				return it.Title, nil
			}
		case float64:
			if int(x) == id && strings.TrimSpace(it.Title) != "" {
				return it.Title, nil
			}
		}
	}
	return idStr, nil
}

// resolvePoliceStationTitle: ใช้ provincialId จาก ctx หรือ ENV แล้วไปยิง external
func resolvePoliceStationTitle(ctx context.Context, provincialID, stationID int) (string, error) {
	if provincialID == 0 || stationID == 0 {
		return "", nil
	}
	base := strings.TrimRight(os.Getenv("WATCHMAN_API"), "/")
	if base == "" {
		return strconv.Itoa(stationID), errors.New("WATCHMAN_API not set")
	}

	url := fmt.Sprintf("%s/api_options.php?f=station&provincialId=%d", base, provincialID)
	client := &http.Client{}
	var lastErr error

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
			continue
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("remote %d: %s", resp.StatusCode, string(b))
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
			continue
		}

		sid := strconv.Itoa(stationID)

		// 1) { "listPoliceStation": [...] }
		var obj struct {
			List []map[string]any `json:"listPoliceStation"`
		}
		if json.Unmarshal(b, &obj) == nil && len(obj.List) > 0 {
			if t := findTitle(obj.List, sid); t != "" {
				return t, nil
			}
			lastErr = fmt.Errorf("station id %s not found (listPoliceStation)", sid)
			continue
		}

		// 2) { "data": [...] }
		var wrap struct {
			Data []map[string]any `json:"data"`
		}
		if json.Unmarshal(b, &wrap) == nil && len(wrap.Data) > 0 {
			if t := findTitle(wrap.Data, sid); t != "" {
				return t, nil
			}
			lastErr = fmt.Errorf("station id %s not found (data)", sid)
			continue
		}

		// 3) [ {...} ]
		var arr []map[string]any
		if json.Unmarshal(b, &arr) == nil && len(arr) > 0 {
			if t := findTitle(arr, sid); t != "" {
				return t, nil
			}
			lastErr = fmt.Errorf("station id %s not found (array)", sid)
			continue
		}

		lastErr = fmt.Errorf("unexpected schema from %s: %s", url, string(b))
	}

	return strconv.Itoa(stationID), lastErr
}

func findTitle(arr []map[string]any, sid string) string {
	for _, it := range arr {
		var idStr string
		if v, ok := it["id"]; ok {
			switch t := v.(type) {
			case string:
				idStr = strings.TrimSpace(t)
			case float64:
				idStr = strconv.Itoa(int(t))
			case int:
				idStr = strconv.Itoa(t)
			}
		}
		title := firstNonEmptyStr(it["title"], it["name"])
		if idStr == sid && title != "" {
			return title
		}
	}
	return ""
}

func firstNonEmptyStr(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		}
	}
	return ""
}

func cleanupOnCreateFail(ctx context.Context, docID primitive.ObjectID, photoKey string) error {
	// 1) ลบ MongoDB
	if docID != primitive.NilObjectID {
		_, _ = stomongo.DeleteByID(ctx, watchlistColl, docID)
	}
	// 2) ลบ S3 ตาม key เดียว (ไม่ไล่ลบทั้งโฟลเดอร์)
	if strings.TrimSpace(photoKey) != "" {
		_ = stos3minio.DeleteByKey(ctx, "kwatch", photoKey)
	}
	return nil
}

func atoiOpt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func setIntIf(m bson.M, field string, p *string) {
	if p == nil {
		return
	}
	if v, ok := atoiOpt(*p); ok {
		m[field] = v
	} else {
		// แนะนำ: return 400 ที่ controller ถ้า numeric ไม่ผ่าน (ด้วย validator)
		// หรือข้ามการตั้งค่าได้ถ้าต้องการ soft-fail
	}
}

// ---------- small helpers used by update ----------

func setStrIf(set bson.M, field string, v *string) {
	if v != nil {
		set[field] = *v
	}
}

func choose(ptr *string, old any) string {
	if ptr != nil {
		return *ptr
	}
	return str(old)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func intOf(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(x))
		return i
	default:
		return 0
	}
}

// ใช้กับ pointer ที่เป็น *string -> คืน int
func chooseIntStr(p *string, old any) int {
	if p != nil {
		if i, err := strconv.Atoi(strings.TrimSpace(*p)); err == nil {
			return i
		}
		// ถ้า parse ไม่ได้ ให้คงค่าเดิมจาก DB
	}
	return intOf(old)
}

// string เวอร์ชันเดิม (ปลอดภัย)
func chooseStr(p *string, old any) string {
	if p != nil {
		return strings.TrimSpace(*p)
	}
	return strings.TrimSpace(fmt.Sprint(old))
}
