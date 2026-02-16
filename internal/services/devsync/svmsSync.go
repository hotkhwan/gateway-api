// internal/services/devsync/svmssync.go
package devsync

import (
	"context"
	"strconv"
	"time"

	"github.com/hotkhwan/gateway-api/internal/gateways/svms"
	"github.com/hotkhwan/gateway-api/internal/logger"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SyncConfig struct {
	BaseURL  string
	User     string
	Pass     string
	PageSize int
}

// Sync by url user name and Password from SVMS system]
func SyncFromSVMS(ctx context.Context, cfg SyncConfig, db *mongo.Database) (int, int, error) {
	log := logger.FromCtx(ctx, "devsync", "svmsSync")

	client := svms.New(cfg.BaseURL, cfg.User, cfg.Pass)
	if err := client.Login(ctx); err != nil {
		return 0, 0, err
	}
	log.Info().Msg("SVMS login success")

	coll := db.Collection("camera")

	total := 0
	upserts := 0
	pageNo := 0
	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 200
	}

	for {
		page, err := client.ReadChannelListByPage(ctx, pageNo, pageSize, true)
		if err != nil {
			log.Error().Err(err).Int("pageNo", pageNo).Msg("read channels failed")
			return total, upserts, err
		}
		for _, ch := range page.Items {
			total++

			lat, _ := strconv.ParseFloat(ch.Latitude, 64)
			lng, _ := strconv.ParseFloat(ch.Longitude, 64)

			// สร้างเอกลักษณ์: ใช้ RTSP เป็น key ถ้า stable; ถ้าไม่ ให้ใช้ gbid+channel หรือ parent+name
			key := ch.RTSP
			if key == "" {
				key = ch.GBID + "::" + strconv.Itoa(ch.Channel)
			}

			doc := bson.M{
				"name":           ch.Name,
				"url":            ch.RTSP,  // ใช้ RTSP เป็นหลัก
				"district":       ch.PName, // map parent name เป็น district ถ้าคุณใช้แบบนี้
				"lat":            lat,
				"lng":            lng,
				"brand":          "KSVMS",       // หรือ map จาก plugin/parent ถ้ามี
				"status":         ch.Valid == 1, // online=1
				"state":          "synced",
				"gbid":           ch.GBID,
				"channel":        ch.Channel,
				"dateTimeUpdate": time.Now(),
			}

			// Upsert: filter ตาม key
			filter := bson.M{"key": key}
			update := bson.M{
				"$set": doc,
				"$setOnInsert": bson.M{
					"dateTimeCreate": time.Now(),
					"key":            key,
				},
			}
			opts := options.Update().SetUpsert(true)
			res, err := coll.UpdateOne(ctx, filter, update, opts)
			if err != nil {
				log.Error().Err(err).Str("name", ch.Name).Msg("upsert failed")
				continue
			}
			if res.MatchedCount == 0 && res.UpsertedCount == 1 {
				upserts++
			}
		}

		if page.PageNo+1 >= page.PageCount || len(page.Items) == 0 {
			break
		}
		pageNo++
	}

	return total, upserts, nil
}
