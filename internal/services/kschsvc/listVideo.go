// internal/services/kschsvc/listVideo.go
package kschsvc

import (
	"context"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/models/repomod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// โครงสร้างที่ตอบกลับ API (เพิ่ม video_url สำหรับ presign)

// VideoList ดึงรายการวิดีโอ พร้อม presign URL แบบรวดเดียว
// filters รองรับ: id, search, state, status ("true"/"false")
func VideoList(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]kschmod.VideoResponse, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kschsvc",    // tracerName
		"search.VideoList", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kschsvc", "VideoList",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// base filter (state = created เป็นค่า default ถ้าไม่ส่งมา)
	filter := bson.M{}
	if v := filters["state"]; v != "" {
		filter["state"] = v
	} else {
		filter["state"] = "created"
	}

	if v := filters["status"]; v != "" {
		switch v {
		case "1", "true", "t", "yes", "y", "on":
			filter["status"] = true
		case "0", "false", "f", "no", "n", "off":
			filter["status"] = false
		}
	}
	if v := filters["search"]; v != "" {
		// ค้นหาจากชื่อ (name)
		filter["name"] = bson.M{"$regex": v, "$options": "i"}
	}

	// pagination & sort
	if page <= 0 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 20
	}
	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	if sortField == "" {
		sortField = "createdAt"
	}
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}})

	var rows []kschmod.VideoResponse
	if err := stomongo.Find(ctx, videoColl, filter, opts, &rows); err != nil {
		log.Error().Err(err).Msg("❌ stomongo.Find failed")
		return nil, gmod.Pagination{}, err
	}

	total, err := stomongo.Count(ctx, videoColl, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ stomongo.Count failed")
		return nil, gmod.Pagination{}, err
	}

	// presign video url
	// เลือก bucket จาก ENV ก่อน (เผื่อแยกได้), ไม่งั้น fallback เป็นคงที่ใน service
	bucket := os.Getenv("S3_KSEARCH_BUCKET")
	if bucket == "" {
		// เผื่อใช้ S3_BUCKET รวม
		bucket = os.Getenv("S3_BUCKET")
	}
	if bucket == "" {
		// สุดท้าย fallback ตามที่ service create ใช้
		bucket = videoBucket
	}

	if bucket == "" {
		log.Warn().Msg("⚠️ S3 bucket is empty, skip presign")
	} else {
		keys := make([]string, 0, len(rows))
		seen := make(map[string]struct{}, len(rows))
		for _, v := range rows {
			if k := v.VideoKey; k != "" {
				if _, ok := seen[k]; !ok {
					seen[k] = struct{}{}
					keys = append(keys, k)
				}
			}
		}

		if len(keys) > 0 {
			// expiry=0 (ให้ lib ใช้ default), maxWorkers=10
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
				if k := rows[i].VideoKey; k != "" {
					if it, ok := m[k]; ok && it.URL != "" {
						rows[i].VideoURL = it.URL
					}
				}
			}
		}
	}

	log.Info().
		Int("page", page).
		Int("perPages", perPages).
		Int("total", int(total)).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Msg("✅ Retrieved videos with pagination + presign")

	return rows, gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}, nil
}
