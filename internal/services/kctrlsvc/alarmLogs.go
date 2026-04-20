// internal/services/kctrlsvc/alarmLogs.go
package kctrlsvc

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/hotkhwan/gateway-api/utils/typeutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ListAlarmsLogs(
	ctx context.Context,
	page, perPages int,
	filters map[string]string,
	sortOrder string,
) ([]kctrlmod.AlarmAuditLogItem, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.ListAlarmsLogs", "kctrlsvc", "ListAlarmsLogs")
	defer end()

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if page <= 0 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 20
	}

	sortVal := -1
	if strings.ToLower(strings.TrimSpace(sortOrder)) == "asc" {
		sortVal = 1
	}

	// sortField whitelist (กันโดนยิง field แปลกๆ)
	sortField := strings.TrimSpace(filters["sortField"])
	switch sortField {
	case "createdAt", "status", "method", "actorId":
		// ok
	default:
		sortField = "createdAt"
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("audit_logs")

	f := bson.M{
		"operation": "kcontrolAlarms",
		"resource":  bson.M{"$regex": "^/kcontrol/alarms"},
	}

	if v := strings.TrimSpace(filters["alarmId"]); v != "" {
		// ถ้าคุณอยาก strict ว่าเป็นของ alarmId นั้นจริงๆ ใช้ regex แบบ anchor
		f["resource"] = bson.M{"$regex": "^/kcontrol/alarms/" + v}
	}

	if v := strings.TrimSpace(filters["method"]); v != "" {
		f["method"] = strings.ToUpper(v)
	}
	if v := strings.TrimSpace(filters["actorId"]); v != "" {
		f["actorId"] = v
	}

	// datetime range -> createdAt
	from := strings.TrimSpace(filters["from"])
	to := strings.TrimSpace(filters["to"])

	if from != "" || to != "" {
		createdAt := bson.M{}
		if from != "" {
			t, err := time.Parse(time.RFC3339, from)
			if err == nil {
				createdAt["$gte"] = t
			}
		}
		if to != "" {
			t, err := time.Parse(time.RFC3339, to)
			if err == nil {
				createdAt["$lte"] = t
			}
		}
		if len(createdAt) > 0 {
			f["createdAt"] = createdAt
		}
	}

	skip := int64((page - 1) * perPages)
	limit := int64(perPages)

	findOpts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortVal}}).
		SetSkip(skip).
		SetLimit(limit)

	cur, err := coll.Find(cctx, f, findOpts)
	if err != nil {
		log.Error().Err(err).Msg("find audit_logs failed")
		return nil, gmod.Pagination{}, err
	}
	defer cur.Close(cctx)

	var rows []bson.M
	if err := cur.All(cctx, &rows); err != nil {
		log.Error().Err(err).Msg("decode audit_logs failed")
		return nil, gmod.Pagination{}, err
	}

	out := make([]kctrlmod.AlarmAuditLogItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, kctrlmod.AlarmAuditLogItem{
			Id:        typeutil.ObjectIdHex(r["_id"]),
			ActorType: typeutil.Str(r["actorType"]),
			ActorId:   typeutil.Str(r["actorId"]),
			ActorName: typeutil.Str(r["actorName"]),
			ActorIp:   typeutil.Str(r["actorIp"]),

			Operation: typeutil.Str(r["operation"]),
			Resource:  typeutil.Str(r["resource"]),
			Method:    typeutil.Str(r["method"]),
			Path:      typeutil.Str(r["path"]),
			Status:    typeutil.Int(r["status"]),
			LatencyMs: typeutil.Int64(r["latencyMs"]),

			Payload:   r["payload"],
			TraceId:   typeutil.Str(r["traceId"]),
			CreatedAt: typeutil.Time(r["createdAt"]),
		})
	}

	total, err := coll.CountDocuments(cctx, f)
	if err != nil {
		log.Error().Err(err).Msg("count audit_logs failed")
		return nil, gmod.Pagination{}, err
	}

	pagination := gmod.Pagination{
		Page:         page,
		PerPage:      perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    strings.ToLower(strings.TrimSpace(sortOrder)),
	}

	return out, pagination, nil
}
