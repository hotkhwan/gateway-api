// internal/services/kwatsvc/listWatchlist.go
package kwatsvc

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/repo/stomongo"
	"klynx/internal/repo/stos3minio"
	"klynx/models/kwatmod"
	"klynx/models/repomod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// อนุญาตให้ sort ได้เฉพาะ field ที่ whitelist เพื่อลดความเสี่ยง
	allowedSortFields = map[string]struct{}{
		"updatedAt":    {},
		"createdAt":    {},
		"firstname":    {},
		"lastname":     {},
		"nickname":     {},
		"idcard":       {},
		"personalType": {},
		"type":         {},
		"rev":          {},
	}
)

func sanitizeSortField(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "updatedAt"
	}
	if _, ok := allowedSortFields[in]; ok {
		return in
	}
	return "updatedAt"
}

func WatchlistList(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]kwatmod.WatchlistResponse, kwatmod.WatchlistPagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/kwatsvc",
		"kwatch.WatchlistList",
		"kwatsvc", "WatchlistList",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// ✅ แยก baseFilter (ระดับคน) ออกจาก warrantElemFilter (ระดับ element)
	baseFilter := bson.M{
		"isDeleted": bson.M{"$ne": true},
	}

	// helper: แยกคอมมาแล้ว trim
	splitCSV := func(s string) []string {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if pp := strings.TrimSpace(p); pp != "" {
				out = append(out, pp)
			}
		}
		return out
	}

	// 🔹 optional filters (top-level) → ใส่ที่ baseFilter
	if id := strings.TrimSpace(filters["id"]); id != "" {
		if objID, err := primitive.ObjectIDFromHex(id); err == nil {
			baseFilter["_id"] = objID
		}
	}
	if v := strings.TrimSpace(filters["idcard"]); v != "" {
		baseFilter["idcard"] = v
	}
	if v := strings.TrimSpace(filters["passport"]); v != "" {
		baseFilter["passport"] = v
	}
	if v := strings.TrimSpace(filters["type"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			baseFilter["type"] = n
		}
	}

	// 🔹 warrant-level filters → เก็บไว้ใน elem แยกต่างหาก
	elem := bson.M{}
	if vals := splitCSV(filters["policeRegion"]); len(vals) > 0 {
		if len(vals) == 1 {
			elem["policeRegion"] = vals[0]
		} else {
			elem["policeRegion"] = bson.M{"$in": vals}
		}
	}
	if vals := splitCSV(filters["policeProvincial"]); len(vals) > 0 {
		if len(vals) == 1 {
			elem["policeProvincial"] = vals[0]
		} else {
			elem["policeProvincial"] = bson.M{"$in": vals}
		}
	}
	if vals := splitCSV(filters["policeStation"]); len(vals) > 0 {
		if len(vals) == 1 {
			elem["policeStation"] = prefixRegex(vals[0])
		} else {
			regs := make([]primitive.Regex, 0, len(vals))
			for _, v := range vals {
				regs = append(regs, prefixRegex(v))
			}
			elem["policeStation"] = bson.M{"$in": regs}
		}
	}

	// 🔹 สร้าง $or รวม: สำหรับ search และ state → ใส่ใน baseFilter
	var ors []bson.M
	if v := strings.TrimSpace(filters["search"]); v != "" {
		ors = append(ors,
			bson.M{"personKey": bson.M{"$regex": v, "$options": "i"}},
			bson.M{"firstname": bson.M{"$regex": v, "$options": "i"}},
			bson.M{"lastname": bson.M{"$regex": v, "$options": "i"}},
			bson.M{"nickname": bson.M{"$regex": v, "$options": "i"}},
			bson.M{"idcard": bson.M{"$regex": v, "$options": "i"}},
			bson.M{"passport": bson.M{"$regex": v, "$options": "i"}},
		)
	}
	if sv := strings.TrimSpace(filters["state"]); sv != "" {
		states := splitCSV(sv)
		if len(states) > 0 {
			ors = append(ors,
				bson.M{"state": bson.M{"$in": states}},
				bson.M{"lifecycle.state": bson.M{"$in": states}},
			)
		}
	}
	if len(ors) > 0 {
		baseFilter["$or"] = ors
	}

	// ✅ filter สำหรับ "หาเอกสาร (คน)" = baseFilter + มีอย่างน้อย 1 warrants ตรง elem (ถ้ามี elem)
	filter := bson.M{}
	for k, v := range baseFilter {
		filter[k] = v
	}
	if len(elem) > 0 {
		filter["warrants"] = bson.M{"$elemMatch": elem}
	}

	// 🔹 pagination & sort (เหมือนเดิม)
	if page <= 0 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}
	skip := (page - 1) * perPages

	sf := sanitizeSortField(sortField)
	dir := -1
	if strings.ToLower(sortOrder) == "asc" {
		dir = 1
	}
	sortDoc := bson.D{{Key: sf, Value: dir}}
	if sf != "_id" {
		sortDoc = append(sortDoc, bson.E{Key: "_id", Value: dir})
	}
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(sortDoc)

	// ✅ query data
	var rows []kwatmod.WatchlistResponse
	if err := stomongo.Find(ctx, "kwatch_watchlist", filter, opts, &rows); err != nil {
		log.Error().Err(err).Msg("❌ stomongo.Find failed")
		return nil, kwatmod.WatchlistPagination{}, err
	}

	// ✅ นับหมายจับของแต่ละคนแบบไม่พังแม้ Warrants จะเป็น any
	for i := range rows {
		rows[i].WarrantsPerson = countWarrantsAny(rows[i].Warrants)
	}

	// ✅ นับคน (persons)
	totalPersons64, err := stomongo.Count(ctx, "kwatch_watchlist", filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ stomongo.Count failed")
		return nil, kwatmod.WatchlistPagination{}, err
	}
	totalPersons := int(totalPersons64)

	// ✅ นับภาพใบหน้า (เฉพาะที่มี photoFaceKey)
	faceFilter := bson.M{}
	for k, v := range filter {
		faceFilter[k] = v
	}
	faceFilter["photoFaceKey"] = bson.M{"$nin": []any{"", nil}}

	totalFaceImg64, err := stomongo.Count(ctx, "kwatch_watchlist", faceFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ stomongo.Count (photoFaceKey) failed")
		return nil, kwatmod.WatchlistPagination{}, err
	}
	totalFaceImg := int(totalFaceImg64)

	// ✅ นับหมายจับ (warrants) ให้ตรงตามฟิลเตอร์ระดับ element จริง ๆ
	// pipeline: match baseFilter → unwind warrants → (ถ้ามี elem) match เฉพาะ warrants ที่ตรง → count
	// หมายเหตุ: ถ้าไม่มี elem เลย แปลว่าไม่ได้กรองระดับ warrants → นับ “ทุกใบ” ของคนที่ผ่าน baseFilter
	warrantMatch := bson.M{}
	for k, v := range elem {
		warrantMatch["warrants."+k] = v
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: baseFilter}},
		bson.D{{Key: "$unwind", Value: "$warrants"}},
	}
	if len(warrantMatch) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: warrantMatch}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$count", Value: "totalWarrants"}})

	totalWarrants := 0
	{
		var aggRes []struct {
			TotalWarrants int `bson:"totalWarrants"`
		}
		if err := stomongo.Aggregate(ctx, "kwatch_watchlist", pipeline, &aggRes); err != nil {
			log.Error().Err(err).Interface("pipeline", pipeline).Msg("❌ aggregate failed")
			return nil, kwatmod.WatchlistPagination{}, err
		}
		if len(aggRes) > 0 {
			totalWarrants = aggRes[0].TotalWarrants
		}
	}

	// 🔹 presign รูป (เดิม)
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("S3_WATCHLIST_BUCKET")
	}
	if bucket != "" && len(rows) > 0 {
		keys := make([]string, 0, len(rows))
		seen := make(map[string]struct{}, len(rows))
		for _, w := range rows {
			if k := strings.TrimSpace(w.PhotoKey); k != "" {
				if _, ok := seen[k]; !ok {
					seen[k] = struct{}{}
					keys = append(keys, k)
				}
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			items := stos3minio.PresignMany(ctx, config.S3Client, bucket, keys, 0, 10)
			m := make(map[string]repomod.PresignItem, len(items))
			for _, it := range items {
				if it.Err != nil {
					it.ErrString = it.Err.Error()
					log.Warn().Str("key", it.Key).Err(it.Err).Msg("⚠️ presign failed")
				}
				m[it.Key] = it
			}
			for i := range rows {
				if k := strings.TrimSpace(rows[i].PhotoKey); k != "" {
					if it, ok := m[k]; ok && it.URL != "" {
						rows[i].PhotoURL = it.URL
					}
				}
			}
		}
	} else if bucket == "" {
		log.Warn().Msg("⚠️ S3 bucket empty, skip presign")
	}

	log.Info().
		Int("page", page).
		Int("perPages", perPages).
		Int("totalFaceImages", totalFaceImg).
		Int("totalPersons", totalPersons).
		Int("totalWarrants", totalWarrants).
		Str("sortField", sf).
		Str("sortOrder", map[int]string{1: "asc", -1: "desc"}[dir]).
		Interface("filter.person", baseFilter).
		Interface("filter.warrantElem", elem).
		Msg("✅ Watchlist list fetched")

	return rows, kwatmod.WatchlistPagination{
		Page:            page,
		PerPages:        perPages,
		TotalRecords:    totalPersons, // คงพฤติกรรมเดิม
		TotalPages:      (totalPersons + perPages - 1) / perPages,
		SortField:       sf,
		SortOrder:       map[int]string{1: "asc", -1: "desc"}[dir],
		TotalFaceImages: totalFaceImg,
		TotalPersons:    totalPersons,  // ✅ ใหม่
		TotalWarrants:   totalWarrants, // ✅ ใหม่
	}, nil
}

// helper: ทำ regex แบบ prefix (escape ไว้ กัน .()*+? ฯลฯ)
func prefixRegex(s string) primitive.Regex {
	s = strings.TrimSpace(s)
	return primitive.Regex{
		Pattern: "^" + regexp.QuoteMeta(s),
		Options: "i", // ไทยไม่สน case แต่ใส่ไว้เผื่อข้อมูลปนอังกฤษ
	}
}

func countWarrantsAny(v any) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return rv.Len()
	default:
		return 0
	}
}
