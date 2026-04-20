// internal/services/kctrlsvc/alarm.go
package kctrlsvc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kcontrolmsg"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func ListAlarms(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]kctrlmod.AlarmResponseItem, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kctrlsvc",
		"alarms.ListAlarms",
		"kctrlsvc", "ListAlarms",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().
		Int("page", page).
		Int("perPage", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Msg("📥 Fetching list of alarms (exclude acknowledged)")

	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol_alarms") // คงตามเดิม

	// ฟิลเตอร์หลัก: ตัด acknowledged=true ออก (รวมถึงกรณีไม่มีฟิลด์ acknowledged)
	baseFilter := bson.M{
		// "status": bson.M{"$ne": "resolved"},
		// "acknowledged": bson.M{"$ne": true},
		"isDeleted": bson.M{"$ne": true},
	}
	if filters["unacked"] == "true" {
		// ❌ ตัด acknowledged:true
		// ✅ เอา false + null + missing
		baseFilter["acknowledged"] = bson.M{"$ne": true}
	} else {
		// default behavior
		// ✅ เอาเฉพาะ acknowledged:true
		baseFilter["acknowledged"] = true
	}
	// (ออปชัน) รองรับตัวกรองเพิ่มเติมที่อาจส่งมาผ่าน filters เช่น hwId, brand, topic, district ฯลฯ
	// ✅ hwId filter (รองรับทั้ง hwId/hwid กันพลาด)
	if v, ok := filters["hwId"]; ok && strings.TrimSpace(v) != "" {
		baseFilter["hwId"] = strings.TrimSpace(v)
	} else if v, ok := filters["hwid"]; ok && strings.TrimSpace(v) != "" {
		baseFilter["hwId"] = strings.TrimSpace(v)
	}

	// ✅ search: message OR name OR hwId (case-insensitive)
	if s, ok := filters["search"]; ok && strings.TrimSpace(s) != "" {
		q := strings.TrimSpace(s)
		baseFilter["$or"] = []bson.M{
			{"message": bson.M{"$regex": q, "$options": "i"}},
			{"name": bson.M{"$regex": q, "$options": "i"}},
			{"hwId": bson.M{"$regex": q, "$options": "i"}},
		}
	}
	if v, ok := filters["brand"]; ok && v != "" {
		baseFilter["brand"] = v
	}
	if v, ok := filters["topic"]; ok && v != "" {
		baseFilter["topic"] = v
	}
	if v, ok := filters["district"]; ok && v != "" {
		baseFilter["district"] = v
	}
	// ตัวอย่างช่วงเวลา (ISO8601) filters["from"], filters["to"]
	if from, ok := filters["from"]; ok && from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			if baseFilter["datetime"] == nil {
				baseFilter["datetime"] = bson.M{}
			}
			baseFilter["datetime"].(bson.M)["$gte"] = t
		}
	}
	if to, ok := filters["to"]; ok && to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			if baseFilter["datetime"] == nil {
				baseFilter["datetime"] = bson.M{}
			}
			baseFilter["datetime"].(bson.M)["$lte"] = t
		}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: baseFilter}},
		{{Key: "$sort", Value: bson.D{{Key: "datetime", Value: sortVal}}}},
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: perPages}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("❌ Aggregate pipeline execution failed")
		return nil, gmod.Pagination{}, err
	}
	defer cursor.Close(ctx)

	var raw []bson.M
	if err := cursor.All(ctx, &raw); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode aggregation result")
		return nil, gmod.Pagination{}, err
	}

	log.Debug().Int("alarm_count", len(raw)).Msg("✅ Aggregated alarm records fetched")

	// map ออกเป็น response items (กันพังหากบางฟิลด์ไม่มี)
	var alarms []kctrlmod.AlarmResponseItem
	toIntPtr := func(v any) *int {
		switch t := v.(type) {
		case int:
			return &t
		case int32:
			tt := int(t)
			return &tt
		case int64:
			tt := int(t)
			return &tt
		default:
			return nil
		}
	}

	toInt64Ptr := func(v any) *int64 {
		switch t := v.(type) {
		case int64:
			return &t
		case int32:
			tt := int64(t)
			return &tt
		case int:
			tt := int64(t)
			return &tt
		default:
			return nil
		}
	}

	toFloat64Ptr := func(v any) *float64 {
		switch t := v.(type) {
		case float64:
			return &t
		case float32:
			tt := float64(t)
			return &tt
		case int:
			tt := float64(t)
			return &tt
		case int32:
			tt := float64(t)
			return &tt
		case int64:
			tt := float64(t)
			return &tt
		default:
			return nil
		}
	}

	toRFC3339 := func(v any) string {
		if dt, ok := v.(primitive.DateTime); ok {
			return dt.Time().Format(time.RFC3339)
		}
		if t, ok := v.(time.Time); ok {
			return t.Format(time.RFC3339)
		}
		return ""
	}

	for _, r := range raw {
		var item kctrlmod.AlarmResponseItem

		if id, ok := r["_id"].(primitive.ObjectID); ok {
			item.ID = id
		}
		if msg, ok := r["message"].(string); ok {
			item.Message = msg
		}
		item.Datetime = toRFC3339(r["datetime"])

		if name, ok := r["name"].(string); ok {
			item.Name = name
		}
		if hw, ok := r["hwId"].(string); ok {
			item.HwID = hw
		}
		if d, ok := r["district"].(string); ok {
			item.District = d
		}
		if lat, ok := r["lat"].(float64); ok {
			item.Lat = lat
		} else if latf, ok := r["lat"].(float32); ok {
			item.Lat = float64(latf)
		}
		if lng, ok := r["lng"].(float64); ok {
			item.Lng = lng
		} else if lngf, ok := r["lng"].(float32); ok {
			item.Lng = float64(lngf)
		}
		if brand, ok := r["brand"].(string); ok {
			item.Brand = brand
		}

		// ✅ FLAT alarm fields (ย้ายออกจาก detail)
		if v, ok := r["status"].(string); ok {
			item.Status = v
		}
		if v, ok := r["cycleId"].(string); ok {
			item.CycleId = v
		}

		item.Sensor = toIntPtr(r["sensor"])
		item.StartedAt = toInt64Ptr(r["startedAt"])
		item.AlarmAt = toInt64Ptr(r["alarmAt"])
		item.Elapsed = toFloat64Ptr(r["elapsed"])

		if v, ok := r["acknowledged"].(bool); ok {
			item.Acknowledged = &v
		}
		item.AcknowledgedAt = toRFC3339(r["acknowledgedAt"])

		if ab, ok := r["acknowledgedBy"].(bson.M); ok {
			actor := &kctrlmod.AlarmActor{}
			if s, ok := ab["actorId"].(string); ok {
				actor.ActorId = s
			}
			if s, ok := ab["actorName"].(string); ok {
				actor.ActorName = s
			}
			if s, ok := ab["actorIp"].(string); ok {
				actor.ActorIp = s
			}
			// ใส่เฉพาะถ้ามีค่าอย่างน้อย 1 อัน
			if actor.ActorId != "" || actor.ActorName != "" || actor.ActorIp != "" {
				item.AcknowledgedBy = actor
			}
		}

		// sop: [] (array of objects) -> flat item.Sop
		if sopRaw, ok := r["sop"].(bson.A); ok {
			steps := make([]kctrlmod.AlarmSopStep, 0, len(sopRaw))
			for _, x := range sopRaw {
				m, ok := x.(bson.M)
				if !ok {
					continue
				}

				var st kctrlmod.AlarmSopStep

				if oid, ok := m["stepId"].(primitive.ObjectID); ok {
					st.StepId = oid
				}
				if s, ok := m["type"].(string); ok {
					st.Type = s
				}
				if s, ok := m["description"].(string); ok {
					st.Description = s
				}
				if s, ok := m["stepStatus"].(string); ok {
					st.StepStatus = s
				}

				st.CreatedAt = toRFC3339(m["createdAt"])

				if d, ok := m["detail"].(bson.M); ok {
					st.Detail = d
				} else if d, ok := m["detail"].(map[string]any); ok {
					st.Detail = d
				}

				if s, ok := m["actorId"].(string); ok {
					st.ActorId = s
				}
				if s, ok := m["actorName"].(string); ok {
					st.ActorName = s
				}
				if s, ok := m["actorIp"].(string); ok {
					st.ActorIp = s
				}

				steps = append(steps, st)
			}
			if len(steps) > 0 {
				item.Sop = steps
			}
		}

		alarms = append(alarms, item)
	}

	// นับรวมด้วยฟิลเตอร์เดียวกัน (ไม่รวม acknowledged=true)
	total, err := coll.CountDocuments(ctx, baseFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count alarm documents")
		return nil, gmod.Pagination{}, err
	}

	pagination := gmod.Pagination{
		Page:         page,
		PerPage:      perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	return alarms, pagination, nil
}

func GetAlarmById(ctx context.Context, id string) (kctrlmod.AlarmResponseItem, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.GetAlarmById", "kctrlsvc", "GetAlarmById")
	defer end()

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(strings.TrimSpace(id))
	if err != nil {
		return kctrlmod.AlarmResponseItem{}, errors.New("invalid ObjectID")
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("kcontrol_alarms")

	// exclude soft-deleted
	filter := bson.M{
		"_id":       objID,
		"isDeleted": bson.M{"$ne": true},
	}

	var r bson.M
	if err := coll.FindOne(cctx, filter).Decode(&r); err != nil {
		if err == mongo.ErrNoDocuments {
			return kctrlmod.AlarmResponseItem{}, errors.New("alarm not found")
		}
		return kctrlmod.AlarmResponseItem{}, err
	}

	// mapping ให้เหมือน ListAlarms (คัด helper ที่จำเป็นมาใช้ซ้ำในฟังก์ชันนี้)
	toIntPtr := func(v any) *int {
		switch t := v.(type) {
		case int:
			return &t
		case int32:
			tt := int(t)
			return &tt
		case int64:
			tt := int(t)
			return &tt
		default:
			return nil
		}
	}

	toInt64Ptr := func(v any) *int64 {
		switch t := v.(type) {
		case int64:
			return &t
		case int32:
			tt := int64(t)
			return &tt
		case int:
			tt := int64(t)
			return &tt
		default:
			return nil
		}
	}

	toFloat64Ptr := func(v any) *float64 {
		switch t := v.(type) {
		case float64:
			return &t
		case float32:
			tt := float64(t)
			return &tt
		case int:
			tt := float64(t)
			return &tt
		case int32:
			tt := float64(t)
			return &tt
		case int64:
			tt := float64(t)
			return &tt
		default:
			return nil
		}
	}

	toRFC3339 := func(v any) string {
		if dt, ok := v.(primitive.DateTime); ok {
			return dt.Time().Format(time.RFC3339)
		}
		if t, ok := v.(time.Time); ok {
			return t.Format(time.RFC3339)
		}
		return ""
	}

	var item kctrlmod.AlarmResponseItem

	if oid, ok := r["_id"].(primitive.ObjectID); ok {
		item.ID = oid
	}
	if v, ok := r["message"].(string); ok {
		item.Message = v
	}
	item.Datetime = toRFC3339(r["datetime"])

	if v, ok := r["name"].(string); ok {
		item.Name = v
	}
	if v, ok := r["hwId"].(string); ok {
		item.HwID = v
	}
	if v, ok := r["district"].(string); ok {
		item.District = v
	}

	// lat/lng รองรับทั้ง float64/float32
	if lat, ok := r["lat"].(float64); ok {
		item.Lat = lat
	} else if latf, ok := r["lat"].(float32); ok {
		item.Lat = float64(latf)
	}
	if lng, ok := r["lng"].(float64); ok {
		item.Lng = lng
	} else if lngf, ok := r["lng"].(float32); ok {
		item.Lng = float64(lngf)
	}

	if v, ok := r["brand"].(string); ok {
		item.Brand = v
	}

	// flat fields
	if v, ok := r["status"].(string); ok {
		item.Status = v
	}
	if v, ok := r["cycleId"].(string); ok {
		item.CycleId = v
	}

	item.Sensor = toIntPtr(r["sensor"])
	item.StartedAt = toInt64Ptr(r["startedAt"])
	item.AlarmAt = toInt64Ptr(r["alarmAt"])
	item.Elapsed = toFloat64Ptr(r["elapsed"])

	if v, ok := r["acknowledged"].(bool); ok {
		item.Acknowledged = &v
	}
	item.AcknowledgedAt = toRFC3339(r["acknowledgedAt"])

	if ab, ok := r["acknowledgedBy"].(bson.M); ok {
		actor := &kctrlmod.AlarmActor{}
		if s, ok := ab["actorId"].(string); ok {
			actor.ActorId = s
		}
		if s, ok := ab["actorName"].(string); ok {
			actor.ActorName = s
		}
		if s, ok := ab["actorIp"].(string); ok {
			actor.ActorIp = s
		}
		if actor.ActorId != "" || actor.ActorName != "" || actor.ActorIp != "" {
			item.AcknowledgedBy = actor
		}
	}

	// sop array
	if sopRaw, ok := r["sop"].(bson.A); ok {
		steps := make([]kctrlmod.AlarmSopStep, 0, len(sopRaw))
		for _, x := range sopRaw {
			m, ok := x.(bson.M)
			if !ok {
				continue
			}

			var st kctrlmod.AlarmSopStep
			if oid, ok := m["stepId"].(primitive.ObjectID); ok {
				st.StepId = oid
			}
			if s, ok := m["type"].(string); ok {
				st.Type = s
			}
			if s, ok := m["description"].(string); ok {
				st.Description = s
			}
			if s, ok := m["stepStatus"].(string); ok {
				st.StepStatus = s
			}

			st.CreatedAt = toRFC3339(m["createdAt"])

			if d, ok := m["detail"].(bson.M); ok {
				st.Detail = d
			} else if d, ok := m["detail"].(map[string]any); ok {
				st.Detail = d
			}

			if s, ok := m["actorId"].(string); ok {
				st.ActorId = s
			}
			if s, ok := m["actorName"].(string); ok {
				st.ActorName = s
			}
			if s, ok := m["actorIp"].(string); ok {
				st.ActorIp = s
			}

			steps = append(steps, st)
		}
		if len(steps) > 0 {
			item.Sop = steps
		}
	}

	log.Debug().Str("id", id).Msg("✅ Alarm fetched by id")
	return item, nil
}

func DeleteAlarm(ctx context.Context, id string) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.DeleteAlarm", "kctrlsvc", "DeleteAlarm")
	defer end()

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().Str("id", id).Msg("🗑️ Soft deleting alarm")

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Warn().Err(err).Msg("invalid ObjectID")
		return errors.New("invalid ObjectID")
	}

	collName := "kcontrol_alarms"

	// ✅ soft delete: isDeleted=true + state=archived + deletedAt/updatedAt
	filter := bson.M{
		"_id":       objID,
		"isDeleted": bson.M{"$ne": true}, // กันกดซ้ำ
	}

	opts := stomongo.SoftDeleteOptions{
		State: "archived",
		ExtraSet: bson.M{
			"status": "deleted", // ถ้าคุณมี status เดิม active/acknowledged
		},
	}

	res, err := stomongo.SoftDeleteOne(cctx, collName, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("soft delete failed")
		return err
	}

	if res.MatchedCount == 0 {
		log.Warn().Str("id", id).Msg("no alarm found to delete (already deleted or not found)")
		return errors.New("no alarm found to delete")
	}

	log.Debug().
		Str("id", id).
		Int64("matched", res.MatchedCount).
		Int64("modified", res.ModifiedCount).
		Msg("✅ Alarm soft deleted")

	return nil
}

// func DeleteAlarm(ctx context.Context, id string) error {
// 	ctx, end, log := traceutil.StartLite(
// 		ctx,
// 		"github.com/hotkhwan/gateway-api/kctrlsvc",     // tracerName
// 		"alarms.DeleteAlarm", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
// 		"kctrlsvc", "LiDeleteAlarmtAlarms",
// 	)
// 	defer end()
// 	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
// 	defer cancel()
// 	log.Debug().Str("id", id).Msg("🗑️ Deleting (hard) alarm")

// 	objID, err := primitive.ObjectIDFromHex(id)
// 	if err != nil {
// 		log.Warn().Err(err).Msg("⚠️ Invalid ObjectID for delete")
// 		return errors.New("invalid ObjectID")
// 	}

// 	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
// 	coll := db.Collection("kcontrol_alarms")

// 	res, err := coll.DeleteOne(ctx, bson.M{"_id": objID})
// 	if err != nil {
// 		log.Error().Err(err).Str("id", id).Msg("❌ Failed to delete alarm")
// 		return err
// 	}

// 	if res.DeletedCount == 0 {
// 		log.Warn().Str("id", id).Msg("⚠️ No alarm found to delete")
// 		return errors.New("no alarm found to delete")
// 	}

// 	log.Debug().Str("id", id).Msg("✅ Alarm permanently deleted")
// 	return nil
// }

func DeleteAlarmBulk(ctx context.Context, ids []string) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.DeleteAlarmBulk", "kctrlsvc", "DeleteAlarmBulk")
	defer end()

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().Int("count", len(ids)).Msg("🗑️ Soft deleting alarms (bulk)")

	objIDs := make([]primitive.ObjectID, 0, len(ids))
	for _, idStr := range ids {
		objID, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			log.Warn().Str("invalidId", idStr).Msg("skip invalid ObjectID")
			continue
		}
		objIDs = append(objIDs, objID)
	}

	if len(objIDs) == 0 {
		return errors.New("no valid ObjectIDs provided")
	}

	collName := "kcontrol_alarms"

	filter := bson.M{
		"_id":       bson.M{"$in": objIDs},
		"isDeleted": bson.M{"$ne": true},
	}

	opts := stomongo.SoftDeleteOptions{
		State: "archived",
		ExtraSet: bson.M{
			"status": "deleted",
		},
	}

	res, err := stomongo.SoftDeleteMany(cctx, collName, filter, opts)
	if err != nil {
		log.Error().Err(err).Msg("bulk soft delete failed")
		return err
	}

	if res.MatchedCount == 0 {
		return errors.New("no alarm found to delete")
	}

	log.Debug().
		Int64("matched", res.MatchedCount).
		Int64("modified", res.ModifiedCount).
		Msg("✅ Bulk alarm soft delete completed")

	return nil
}

func AckAlarm(ctx context.Context, alarmId string, req kctrlmod.AckAlarmRequest, c fiber.Ctx) error {
	traceCtx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.AckAlarm", "kctrlsvc", "AckAlarm")
	defer end()

	cctx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
	defer cancel()

	objId, err := primitive.ObjectIDFromHex(alarmId)
	if err != nil {
		return err
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	almColl := db.Collection("kcontrol_alarms")
	devColl := db.Collection("kcontrol")

	// read alarm for hwId verify + exists check
	var alarm bson.M
	if err := almColl.FindOne(cctx, bson.M{"_id": objId, "isDeleted": bson.M{"$ne": true}}).Decode(&alarm); err != nil {
		if err == mongo.ErrNoDocuments {
			return errors.New("alarm not found")
		}
		return err
	}

	alarmHwId, _ := alarm["hwId"].(string)
	if strings.TrimSpace(alarmHwId) != "" && strings.TrimSpace(req.HwID) != "" && strings.TrimSpace(alarmHwId) != strings.TrimSpace(req.HwID) {
		return errors.New("hwId mismatch")
	}

	now := time.Now()
	actorId, actorName, actorIp := getActorFromCtx(c)

	// build SOP step (append-only)
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = "รับแจ้งเหตุ"
	}

	step := kctrlmod.SopStep{
		StepId:      primitive.NewObjectID(), // ✅ BSON ObjectID
		Type:        "acknowledge",
		Description: description,
		Detail:      req.Detail,
		StepStatus:  "done",
		CreatedAt:   now,
		ActorId:     actorId,
		ActorName:   actorName,
		ActorIp:     actorIp,
	}

	almUpdate := bson.M{
		"$set": bson.M{
			"acknowledged":   true,
			"acknowledgedAt": now,
			"acknowledgedBy": bson.M{
				"actorId":   actorId,
				"actorName": actorName,
				"actorIp":   actorIp,
			},
			"status":    "acknowledged",
			"updatedAt": now,
		},
		"$push": bson.M{
			"sop": bson.M{
				"$each":  []any{step},
				"$slice": -10,
			},
		},
	}

	// optional: กัน ack ซ้ำ (ถ้าอยาก idempotent ค่อยปรับ)
	filter := bson.M{
		"_id":          objId,
		"isDeleted":    bson.M{"$ne": true},
		"acknowledged": bson.M{"$ne": true},
	}

	res, err := almColl.UpdateOne(cctx, filter, almUpdate)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.New("alarm already acknowledged or not found")
	}

	// clear device alarm flag
	if strings.TrimSpace(req.HwID) != "" {
		devUpdate := bson.M{
			"$set": bson.M{
				"alarm":               false,
				"dateTimeUpdate":      now,
				"alarmInfo.clearedAt": now,
				"alarmInfo.clearedBy": actorName,
			},
		}
		if _, err := devColl.UpdateOne(cctx, bson.M{"hwId": req.HwID}, devUpdate); err != nil {
			return err
		}
	}

	// publish mqtt result (คง behavior เดิมของคุณ แต่เปลี่ยน payload ให้เป็น sop[] append)
	out := map[string]any{
		"id":      alarmId,
		"hwId":    req.HwID,
		"status":  "acknowledged",
		"sopStep": step,
		"acknowledgedBy": map[string]any{
			"actorId":   actorId,
			"actorName": actorName,
			"actorIp":   actorIp,
		},
		"updatedAt": now,
	}

	b, err := json.Marshal(out)
	if err != nil {
		return err
	}

	if err := kcontrolmsg.PublishToMQTT("kcontrol.alarmResult", string(b)); err != nil {
		log.Error().Err(err).Msg("mqtt publish failed")
		return err
	}

	return nil
}

func getActorFromCtx(c fiber.Ctx) (actorId, actorName, actorIp string) {
	actorIp = c.IP()

	if u := c.Locals("user"); u != nil {
		if m, ok := u.(map[string]any); ok {
			if v, ok := m["sub"].(string); ok {
				actorId = v
			}
			if v, ok := m["preferred_username"].(string); ok {
				actorName = v
			}
		}
	}

	return
}
