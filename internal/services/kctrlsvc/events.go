// internal/services/kctrlsvc/events.go
package kctrlsvc

import (
	"context"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func ListEvents(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]kctrlmod.EventResponseItem, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "events.ListEvents", "kctrlsvc", "ListEvents")
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol_events")

	match := bson.M{}
	if v := filters["kind"]; v != "" {
		match["kind"] = v
	}
	if v := filters["hwid"]; v != "" {
		match["hwId"] = v
	}
	if v := filters["search"]; v != "" {
		match["$or"] = []bson.M{
			{"message": bson.M{"$regex": v, "$options": "i"}},
			{"name": bson.M{"$regex": v, "$options": "i"}},
		}
	}

	// ตัวอย่างช่วงเวลา (ISO8601) filters["from"], filters["to"]
	if from, ok := filters["from"]; ok && from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			if match["datetime"] == nil {
				match["datetime"] = bson.M{}
			}
			match["datetime"].(bson.M)["$gte"] = t
		}
	}
	if to, ok := filters["to"]; ok && to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			if match["datetime"] == nil {
				match["datetime"] = bson.M{}
			}
			match["datetime"].(bson.M)["$lte"] = t
		}
	}

	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	if sortField == "" {
		sortField = "datetime"
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: sortField, Value: sortVal}, {Key: "_id", Value: sortVal}}}},
		{{Key: "$skip", Value: (page - 1) * perPages}},
		{{Key: "$limit", Value: perPages}},
	}

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("❌ events aggregate failed")
		return nil, gmod.Pagination{}, err
	}
	defer cur.Close(ctx)

	type row struct {
		ID       primitive.ObjectID `bson:"_id"`
		Kind     string             `bson:"kind"`
		Topic    string             `bson:"topic,omitempty"`
		Message  string             `bson:"message,omitempty"`
		Event    string             `bson:"event,omitempty"`
		Datetime primitive.DateTime `bson:"datetime"`

		Name     string  `bson:"name,omitempty"`
		HwID     string  `bson:"hwId,omitempty"`
		District string  `bson:"district,omitempty"`
		Lat      float64 `bson:"lat,omitempty"`
		Lng      float64 `bson:"lng,omitempty"`
		Brand    string  `bson:"brand,omitempty"`

		Sensor    *int     `bson:"sensor,omitempty"`
		Duration  *float64 `bson:"duration,omitempty"`
		CycleID   string   `bson:"cycleId,omitempty"`
		StartedAt *int64   `bson:"startedAt,omitempty"`
		AlarmAt   *int64   `bson:"alarmAt,omitempty"`
		Elapsed   *float64 `bson:"elapsed,omitempty"`
	}

	var items []row
	if err := cur.All(ctx, &items); err != nil {
		log.Error().Err(err).Msg("❌ decode events failed")
		return nil, gmod.Pagination{}, err
	}

	combined := make([]kctrlmod.EventResponseItem, 0, len(items))
	for _, r := range items {
		combined = append(combined, kctrlmod.EventResponseItem{
			ID:        r.ID,
			Kind:      r.Kind,
			Topic:     r.Topic,
			Message:   r.Message,
			Event:     r.Event,
			Datetime:  r.Datetime.Time().Format(time.RFC3339),
			Name:      r.Name,
			HwID:      r.HwID,
			District:  r.District,
			Lat:       r.Lat,
			Lng:       r.Lng,
			Brand:     r.Brand,
			Sensor:    r.Sensor,
			Duration:  r.Duration,
			CycleID:   r.CycleID,
			StartedAt: r.StartedAt,
			AlarmAt:   r.AlarmAt,
			Elapsed:   r.Elapsed,
		})
	}

	total, err := coll.CountDocuments(ctx, match)
	if err != nil {
		return nil, gmod.Pagination{}, err
	}
	pg := gmod.Pagination{
		Page:         page,
		PerPage:      perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	return combined, pg, nil
}
