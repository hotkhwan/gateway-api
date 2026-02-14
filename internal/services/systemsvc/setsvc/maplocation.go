// internal/services/systemsvc/setsvc/maplocation.go
package setsvc

import (
	"context"
	"time"

	"klynx/models/usrmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const systemSettingDocID = "system.setting"
const optionsCollection = "options"

// GetMapSetting: อ่านค่า mapLocation + zoomLevel จาก options/_id=system.setting
// ถ้าไม่พบ -> default lat/lng = "0","0" และ zoomLevel = 0 (หรือคุณจะตั้ง default 15 ก็ได้)
func GetMapSetting(ctx context.Context, db *mongo.Database) (usrmod.MapSetting, error) {
	out := usrmod.MapSetting{
		MapLocation: usrmod.MapLocation{Lat: "0", Lng: "0"},
		ZoomLevel:   0,
	}

	var opt bson.M
	err := db.Collection(optionsCollection).
		FindOne(ctx, bson.M{"_id": systemSettingDocID}).
		Decode(&opt)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return out, nil
		}
		return out, err
	}

	// mapLocation
	if v, ok := opt["mapLocation"]; ok {
		switch mv := v.(type) {
		case bson.M:
			if s, ok := mv["lat"].(string); ok && s != "" {
				out.MapLocation.Lat = s
			}
			if s, ok := mv["lng"].(string); ok && s != "" {
				out.MapLocation.Lng = s
			}
		case map[string]any:
			if s, ok := mv["lat"].(string); ok && s != "" {
				out.MapLocation.Lat = s
			}
			if s, ok := mv["lng"].(string); ok && s != "" {
				out.MapLocation.Lng = s
			}
		}
	}

	// zoomLevel (mongo อาจ decode เป็น int32/int64/float64)
	if v, ok := opt["zoomLevel"]; ok {
		switch z := v.(type) {
		case int32:
			out.ZoomLevel = int(z)
		case int64:
			out.ZoomLevel = int(z)
		case int:
			out.ZoomLevel = z
		case float64:
			out.ZoomLevel = int(z)
		}
	}

	return out, nil
}

// UpdateMapSetting: upsert ค่า mapLocation + zoomLevel ลง options/_id=<id>
// route ของคุณจะส่ง id อะไรมาก็ได้ แต่โดยทั่วไปคือ "system.setting"
func UpdateMapSetting(ctx context.Context, db *mongo.Database, id string, in usrmod.MapSetting) error {
	if id == "" {
		id = systemSettingDocID
	}

	set := bson.M{
		"mapLocation": bson.M{
			"lat": in.MapLocation.Lat,
			"lng": in.MapLocation.Lng,
		},
		"zoomLevel": in.ZoomLevel,
		"updatedAt": time.Now(),
	}

	_, err := db.Collection(optionsCollection).
		UpdateByID(
			ctx,
			id,
			bson.M{"$set": set},
			options.Update().SetUpsert(true),
		)

	return err
}
