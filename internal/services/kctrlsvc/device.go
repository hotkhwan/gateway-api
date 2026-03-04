// internal/services/kctrlsvc/device.go
package kctrlsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListDevices(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]kctrlmod.Device, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kctrlsvc", // tracerName
		"alarms.ListDevices",                       // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kctrlsvc", "ListDevices",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter := bson.M{}
	if id := filters["id"]; id != "" {
		if objID, err := primitive.ObjectIDFromHex(id); err == nil {
			filter["_id"] = objID
		}
	}
	if hwid := filters["hwid"]; hwid != "" {
		filter["hwid"] = hwid
	}
	if code := filters["code"]; code != "" {
		filter["code"] = code
	}
	if search := filters["search"]; search != "" {
		filter["name"] = bson.M{"$regex": search, "$options": "i"}
	}

	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}

	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}})

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol")

	var rawResults []kctrlmod.Device
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		log.Error().Err(err).Str("operation", "FindDevices").Msg("❌ Failed to query devices from MongoDB")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &rawResults); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode device documents")
		return nil, gmod.Pagination{}, err
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count total devices")
		return nil, gmod.Pagination{}, err
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Int("total", int(total)).
		Str("sortOrder", sortOrder).
		Msg("✅ Retrieved devices with pagination")

	devices := make([]kctrlmod.Device, 0, len(rawResults))
	devices = append(devices, rawResults...)
	// devices := make([]kctrlmod.Device, 0, len(rawResults))
	// for _, d := range rawResults {
	// 	devices = append(devices, d)
	// }

	return devices, gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortOrder:    sortOrder,
	}, nil
}

func UpdateDevice(ctx context.Context, id primitive.ObjectID, payload kctrlmod.UpdatePayload) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kctrlsvc", // tracerName
		"alarms.UpdateDevice",                      // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kctrlsvc", "UpdateDevice",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	update := bson.M{}
	if payload.Name != "" {
		update["name"] = payload.Name
	}
	if payload.District != "" {
		update["district"] = payload.District
	}
	if payload.Owner != "" {
		update["owner"] = payload.Owner
	}
	if payload.Phone != "" {
		update["phone"] = payload.Phone
	}
	if payload.Description != "" {
		update["description"] = payload.Description
	}
	if payload.Lat != 0 {
		update["lat"] = payload.Lat
	}
	if payload.Lng != 0 {
		update["lng"] = payload.Lng
	}
	if payload.Approved != nil {
		update["approved"] = *payload.Approved
	}
	if payload.Alarm != nil {
		update["alarm"] = *payload.Alarm
	}
	if payload.GroupIds != nil {
		update["groupIds"] = payload.GroupIds
	}
	update["dateTimeUpdate"] = time.Now()

	if len(update) == 1 { // only dateTimeUpdate was added
		return errors.New("no fields to update")
	}

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("kcontrol")
	_, err := coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	if err != nil {
		log.Error().
			Err(err).
			Str("mongo_id", id.Hex()).
			Msg("❌ Failed to update device")
		return err
	}

	log.Debug().
		Str("mongo_id", id.Hex()).
		Fields(update).
		Msg("✅ Device updated")
	return nil
}

func ResetStats(ctx context.Context, deviceId string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kctrlsvc", // tracerName
		"alarms.ResetStats",                        // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kctrlsvc", "ResetStats",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	objID, err := primitive.ObjectIDFromHex(deviceId)
	if err != nil {
		log.Error().
			Err(err).
			Str("deviceId", deviceId).
			Msg("❌ Invalid ObjectID")
		return fmt.Errorf("invalid device id: %w", err)
	}

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("kcontrol")

	update := bson.M{
		"$set": bson.M{
			"stats.online":            0,
			"stats.warning":           0,
			"stats.offline":           0,
			"stats.alarm":             0,
			"stats.alarmCount":        0,
			"stats.sensorDetectCount": 0,
			"dateTimeUpdate":          time.Now(),
		},
	}

	_, err = coll.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		log.Error().
			Err(err).
			Str("mongo_id", objID.Hex()).
			Msg("❌ Failed to reset stats")
		return fmt.Errorf("failed to reset stats: %w", err)
	}

	log.Debug().
		Str("mongo_id", objID.Hex()).
		Msg("✅ Stats reset for device")
	return nil
}
