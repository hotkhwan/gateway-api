// internal/services/kwatsvc/syncIboc.go
package kwatsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/repo/stomongo"
	"klynx/models/kwatmod"
	"klynx/utils"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// const emitChunkSize = 500
var emitChunkSize = utils.GetenvInt("KWATCH_BATCHSIZE", 2000)

func ibocProdFilter(mode string) bson.M {
	base := bson.M{
		"isDeleted": bson.M{"$ne": true},
		"state":     bson.M{"$ne": "deleted"},
		"idcard":    bson.M{"$nin": []any{"", nil}},
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "all":
		// all = ทุกคนที่มี idcard (คงเดิม)
		return base
	case "backfill":
		// มี id/face แล้ว แต่ยังไม่มี state/syncedAt
		return merge(base, bson.M{"$and": []bson.M{
			{"external.iboc.id": bson.M{"$nin": []any{"", nil}}},
			{"external.iboc.faceId": bson.M{"$nin": []any{"", nil}}},
		}, "$or": []bson.M{
			{"external.iboc.state": bson.M{"$exists": false}},
			{"external.iboc.state": ""},
			{"external.iboc.syncedAt": bson.M{"$exists": false}},
		}})
	default: // "missing"
		return merge(base, bson.M{"$or": []bson.M{
			{"external.iboc.id": bson.M{"$exists": false}},
			{"external.iboc.id": ""},
			{"external.iboc.faceId": bson.M{"$exists": false}},
			{"external.iboc.faceId": ""},
		}})
	}
}

func ibocDevFilter(mode string) bson.M {
	base := bson.M{
		"isDeleted": bson.M{"$ne": true},
		"state":     bson.M{"$ne": "deleted"},
		"idcard":    bson.M{"$nin": []any{"", nil}},
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "all":
		// ลดงาน: ถ้ามีรูปแล้วและ state = active ก็ข้าม
		return merge(base, bson.M{"$or": []bson.M{
			{"photoFaceKey": bson.M{"$in": []any{"", nil}}},
			{"external.ibocdev.state": bson.M{"$ne": "active"}},
		}})
	case "backfill":
		return merge(base, bson.M{"$and": []bson.M{
			{"external.ibocdev.id": bson.M{"$nin": []any{"", nil}}},
			{"external.ibocdev.faceId": bson.M{"$nin": []any{"", nil}}},
		}, "$or": []bson.M{
			{"external.ibocdev.state": bson.M{"$exists": false}},
			{"external.ibocdev.state": ""},
			{"external.ibocdev.syncedAt": bson.M{"$exists": false}},
		}})
	default: // "missing"
		return merge(base, bson.M{"$or": []bson.M{
			{"external.ibocdev.id": bson.M{"$exists": false}},
			{"external.ibocdev.id": ""},
			{"external.ibocdev.faceId": bson.M{"$exists": false}},
			{"external.ibocdev.faceId": ""},
		}})
	}
}

func merge(m bson.M, with bson.M) bson.M {
	out := bson.M{}
	for k, v := range m {
		out[k] = v
	}
	for k, v := range with {
		out[k] = v
	}
	return out
}

// -------------------- Emit (prod) --------------------

// mode: "missing" (default) | "backfill" | "all"
func EmitSyncIbocTask(ctx context.Context, jobID, mode string) error {
	tr := otel.Tracer("klynx/kwatch")
	ctx, span := tr.Start(ctx, "kwatsvc.EmitSyncIbocTask", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	traceID := traceutil.TraceIDFromCtx(ctx)
	log := logger.FromCtx(ctx, "kwatsvc", "EmitSyncIbocTask").With().
		Str("traceID", traceID).Str("jobID", jobID).Str("mode", mode).Logger()

	filter := ibocProdFilter(mode)
	proj := bson.M{"_id": 1, "idcard": 1}
	opt := options.Find().SetProjection(proj).SetBatchSize(2000)

	var rows []struct {
		ID     any    `bson:"_id"`
		IdCard string `bson:"idcard"`
	}
	if err := stomongo.Find(ctx, watchlistColl, filter, opt, &rows); err != nil {
		return fmt.Errorf("mongo find: %w", err)
	}
	if len(rows) == 0 {
		log.Info().Msg("no candidates; not emitting task")
		NotifyIbocProgressEmpty(jobID)
		return nil
	}

	items := make([]map[string]any, 0, emitChunkSize)
	now := time.Now().UTC()
	chunks := 0

	emit := func(batch []map[string]any) error {
		if len(batch) == 0 {
			return nil
		}
		evt := kwatmod.WatchlistEvent{
			ID:      jobID,
			Event:   "watchlist.iboc.sync.task",
			TraceID: traceID,
			Time:    now.Format(time.RFC3339Nano),
			External: map[string]any{
				"task": map[string]any{
					"items":    batch,
					"defaults": map[string]any{"mode": mode},
				},
			},
		}
		headers := map[string]string{
			"event":   evt.Event,
			"source":  "kwatsvc",
			"schema":  "kwatch/watchlistIbocSyncTask/1",
			"traceId": traceID,
		}
		key := "iboc-sync-task-" + jobID + "-" + batch[0]["id"].(string)
		if err := kafka.PublishEventTo(ctx, "kwatch.watchlist", key, evt, headers); err != nil {
			return fmt.Errorf("kafka publish task chunk: %w", err)
		}
		chunks++
		return nil
	}

	for _, r := range rows {
		idHex := toHexID(r.ID)
		idc := strings.TrimSpace(r.IdCard)
		if idHex == "" || idc == "" {
			continue
		}
		items = append(items, map[string]any{"id": idHex, "idcard": idc})
		if len(items) >= emitChunkSize {
			if err := emit(items); err != nil {
				return err
			}
			items = items[:0]
		}
	}
	if len(items) > 0 {
		if err := emit(items); err != nil {
			return err
		}
	}

	log.Info().Int("chunks", chunks).Msg("iboc task emitted (chunked)")
	return nil
}

// -------------------- Emit (dev) --------------------

// mode: "missing" (default) | "backfill" | "all"
func EmitSyncIbocDevTask(ctx context.Context, jobID, mode string) error {
	tr := otel.Tracer("klynx/kwatch")
	ctx, span := tr.Start(ctx, "kwatsvc.EmitSyncIbocDevTask", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	traceID := traceutil.TraceIDFromCtx(ctx)
	log := logger.FromCtx(ctx, "kwatsvc", "EmitSyncIbocDevTask").With().
		Str("traceID", traceID).Str("jobID", jobID).Str("mode", mode).Logger()

	filter := ibocDevFilter(mode)
	proj := bson.M{"_id": 1, "idcard": 1}
	opt := options.Find().SetProjection(proj).SetBatchSize(2000)

	var rows []struct {
		ID     any    `bson:"_id"`
		IdCard string `bson:"idcard"`
	}
	if err := stomongo.Find(ctx, watchlistColl, filter, opt, &rows); err != nil {
		return fmt.Errorf("mongo find: %w", err)
	}
	if len(rows) == 0 {
		log.Info().Msg("no candidates; not emitting task (ibocdev)")
		return nil
	}

	items := make([]map[string]any, 0, emitChunkSize)
	now := time.Now().UTC()
	chunks := 0

	emit := func(batch []map[string]any) error {
		if len(batch) == 0 {
			return nil
		}
		evt := kwatmod.WatchlistEvent{
			ID:      jobID,
			Event:   "watchlist.ibocdev.sync.task",
			TraceID: traceID,
			Time:    now.Format(time.RFC3339Nano),
			External: map[string]any{
				"task": map[string]any{
					"items":    batch,
					"defaults": map[string]any{"mode": mode},
				},
			},
		}
		headers := map[string]string{
			"event":   evt.Event,
			"source":  "kwatsvc",
			"schema":  "kwatch/watchlistIbocDevSyncTask/1",
			"traceId": traceID,
		}
		key := "ibocdev-sync-task-" + jobID + "-" + batch[0]["id"].(string)
		if err := kafka.PublishEventTo(ctx, "kwatch.watchlist", key, evt, headers); err != nil {
			return fmt.Errorf("kafka publish task chunk: %w", err)
		}
		chunks++
		return nil
	}

	for _, r := range rows {
		idHex := toHexID(r.ID)
		idc := strings.TrimSpace(r.IdCard)
		if idHex == "" || idc == "" {
			continue
		}
		items = append(items, map[string]any{"id": idHex, "idcard": idc})
		if len(items) >= emitChunkSize {
			if err := emit(items); err != nil {
				return err
			}
			items = items[:0]
		}
	}
	if len(items) > 0 {
		if err := emit(items); err != nil {
			return err
		}
	}

	log.Info().Int("chunks", chunks).Msg("ibocdev task emitted (chunked)")
	return nil
}

// toHexID normalizes various ID representations to a hex string.
func toHexID(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case primitive.ObjectID:
		return t.Hex()
	case *primitive.ObjectID:
		if t == nil {
			return ""
		}
		return t.Hex()
	default:
		// last resort
		s := fmt.Sprintf("%v", v)
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, `ObjectID("`)
		s = strings.TrimSuffix(s, `")`)
		s = strings.TrimPrefix(s, "ObjectID(")
		s = strings.TrimSuffix(s, ")")
		return strings.TrimSpace(s)
	}
}
