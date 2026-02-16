// internal/services/mapsvc/camera.go
package mapsvc

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/mapmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// cameraDoc ใช้ map จาก collection "camera"
type cameraDoc struct {
	ID          primitive.ObjectID `bson:"_id"`
	Lat         float64            `bson:"lat"`
	Lng         float64            `bson:"lng"`
	URL         string             `bson:"url"`
	IP          string             `bson:"ip"`
	Name        string             `bson:"name"`
	Description string             `bson:"description"`
	Groups      string             `bson:"groups"`
	AtaWsFlvUrl string             `bson:"ataWsFlvUrl"`
	Brand       string             `bson:"brand"`
	Status      bool               `bson:"status"`
	IsDeleted   bool               `bson:"isDeleted,omitempty"` // ✅ ADD
}

// GetDevicesMap ดึง device ตาม viewport แล้วเลือกว่าจะ return เป็น clusters หรือ devices
//   - hasBounds = true → filter ตาม lat/Lng
//   - hasBounds = false → ดึงทั้งหมด (ควรใช้กับระบบที่จำนวน device ยังไม่เยอะมาก)
//   - zoom <= 12 → ทำ clustering (devices ว่าง)
//   - zoom > 12 → ส่ง devices เต็ม (clusters ว่าง)
func GetDevicesMap(
	ctx context.Context,
	hasBounds bool,
	minLat, maxLat float64,
	minLng, maxLng float64,
	zoom int,
) (mapmod.MapDetails, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/mapsvc",
		"maps.GetDevicesMap",
		"mapsvc", "GetDevicesMap",
	)
	defer end()

	dbName := os.Getenv("MONGO_DB")
	db := config.MongoClient.Database(dbName)
	coll := db.Collection("camera")

	// limit timeout กันพลาด
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	filter := bson.M{
		"$or": bson.A{
			bson.M{"isDeleted": bson.M{"$exists": false}},
			bson.M{"isDeleted": false},
		},
	}
	if hasBounds {
		filter["lat"] = bson.M{
			"$gte": minLat,
			"$lte": maxLat,
		}
		filter["lng"] = bson.M{
			"$gte": minLng,
			"$lte": maxLng,
		}
	}

	findOpts := options.Find().
		SetProjection(bson.M{
			"lat":         1,
			"lng":         1,
			"url":         1,
			"ip":          1,
			"name":        1,
			"description": 1,
			"groups":      1,
			"ataWsFlvUrl": 1,
			"brand":       1,
			"status":      1,
			"isDeleted":   1,
		})

	cur, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		log.Error().Err(err).Str("db", dbName).Interface("filter", filter).Msg("❌ coll.Find camera failed")
		return mapmod.MapDetails{}, err
	}
	defer cur.Close(ctx)

	var docs []cameraDoc
	if err := cur.All(ctx, &docs); err != nil {
		log.Error().Err(err).Msg("❌ cursor.All camera failed")
		return mapmod.MapDetails{}, err
	}

	devices := make([]mapmod.MapDeviceItem, 0, len(docs))
	for _, d := range docs {
		// ถ้า lat/Lng เป็น 0,0 และคุณไม่อยากแสดง สามารถ filter ทิ้งได้
		devices = append(devices, mapmod.MapDeviceItem{
			ID:          d.ID.Hex(),
			Lat:         d.Lat,
			Lng:         d.Lng,
			URL:         d.URL,
			IP:          d.IP,
			Name:        d.Name,
			Description: d.Description,
			Groups:      d.Groups,
			AtaWsFlvUrl: d.AtaWsFlvUrl,
			Brand:       d.Brand,
			Status:      d.Status,
		})
	}

	details := mapmod.MapDetails{
		Clusters: []mapmod.MapClusterItem{},
		Devices:  []mapmod.MapDeviceItem{},
	}

	// rule:
	// - zoom <= 12 และจำนวน device เยอะ -> cluster อย่างเดียว
	// - zoom > 12 -> แสดง device รายตัว
	if zoom <= 12 && len(devices) > 0 {
		clusters := clusterDevices(devices, zoom)
		details.Clusters = clusters
		// devices เว้นว่าง ลด data ที่ต้องส่ง
	} else {
		details.Devices = devices
	}

	log.Info().
		Int("zoom", zoom).
		Int("deviceCount", len(devices)).
		Int("clusterCount", len(details.Clusters)).
		Bool("hasBounds", hasBounds).
		Msg("✅ GetDevicesMap completed")

	return details, nil
}

// ---------------- clustering helper ----------------

type clusterAccum struct {
	sumLat float64
	sumLng float64
	count  int
}

func clusterDevices(devs []mapmod.MapDeviceItem, zoom int) []mapmod.MapClusterItem {
	if len(devs) == 0 {
		return nil
	}

	// กำหนด cellSize แบบคร่าว ๆ ตาม zoom
	// zoom น้อย (มองไกล) → cell ใหญ่ → cluster แรง
	// zoom มาก (มองใกล้) → cell เล็กลง
	var cell float64
	switch {
	case zoom <= 5:
		cell = 5.0
	case zoom <= 8:
		cell = 1.0
	case zoom <= 10:
		cell = 0.5
	case zoom <= 12:
		cell = 0.1
	default:
		// ถ้า zoom มากกว่า 12 ปกติเราไม่ใช้ clustering อยู่แล้ว
		cell = 0.05
	}

	m := make(map[string]*clusterAccum)

	for _, d := range devs {
		// ป้องกัน NaN
		if math.IsNaN(d.Lat) || math.IsNaN(d.Lng) {
			continue
		}

		latKey := math.Floor(d.Lat/cell) * cell
		lngKey := math.Floor(d.Lng/cell) * cell

		key := fmt.Sprintf("%.5f_%.5f", latKey, lngKey)

		acc, ok := m[key]
		if !ok {
			acc = &clusterAccum{}
			m[key] = acc
		}
		acc.sumLat += d.Lat
		acc.sumLng += d.Lng
		acc.count++
	}

	clusters := make([]mapmod.MapClusterItem, 0, len(m))
	for _, acc := range m {
		if acc.count == 0 {
			continue
		}
		clusters = append(clusters, mapmod.MapClusterItem{
			Lat:   acc.sumLat / float64(acc.count),
			Lng:   acc.sumLng / float64(acc.count),
			Count: acc.count,
		})
	}

	return clusters
}
