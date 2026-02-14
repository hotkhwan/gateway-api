// internal/services/kwatsvc/syncIbocById.go
package kwatsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"klynx/internal/logger"
	"klynx/internal/repo/stomongo"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// -------------------- Sync by personKeys (prod) --------------------
// NOTE: ไม่ส่ง Kafka task — เรียก HandleSyncIbocTask ตรง ๆ เป็นชุด ๆ
func EmitSyncIbocTaskByIDs(ctx context.Context, jobID, mode string, personKeys []string) error {
	tr := otel.Tracer("klynx/kwatch")
	ctx, span := tr.Start(ctx, "kwatsvc.EmitSyncIbocTaskByPersonKeys", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	traceID := traceutil.TraceIDFromCtx(ctx)
	log := logger.FromCtx(ctx, "kwatsvc", "EmitSyncIbocTaskByPersonKeys").With().
		Str("traceID", traceID).Str("jobID", jobID).Int("personKeys", len(personKeys)).Logger()

	keys := compactKeys(personKeys)
	if len(keys) == 0 {
		NotifyIbocProgressEmpty(jobID)
		log.Info().Msg("no valid personKeys")
		return nil
	}

	// ดึงข้อมูลขั้นต่ำ (_id + idcard) จาก personKey
	filter := bson.M{
		"personKey": bson.M{"$in": keys},
		"isDeleted": bson.M{"$ne": true},
		"state":     bson.M{"$ne": "deleted"},
	}
	proj := bson.M{"_id": 1, "idcard": 1}
	opt := options.Find().SetProjection(proj).SetBatchSize(2000)

	var rows []struct {
		ID     any    `bson:"_id"`
		IdCard string `bson:"idcard"`
	}
	if err := stomongo.Find(ctx, watchlistColl, filter, opt, &rows); err != nil {
		return fmt.Errorf("mongo find by personKey: %w", err)
	}
	if len(rows) == 0 {
		NotifyIbocProgressEmpty(jobID)
		log.Info().Msg("no candidates (by personKey)")
		return nil
	}

	// chunk → เรียก HandleSyncIbocTask โดยตรง (no Kafka)
	now := time.Now().UTC()
	if mode == "" {
		mode = "byids"
	}
	chunks := 0

	itemsAny := make([]any, 0, emitChunkSize) // ← ต้องเป็น []any เพื่อให้ consumer assert ได้

	flush := func() error {
		if len(itemsAny) == 0 {
			return nil
		}
		evt := kwatmod.WatchlistEvent{
			ID:      jobID,
			Event:   "watchlist.iboc.sync.task",
			TraceID: traceID,
			Time:    now.Format(time.RFC3339Nano),
			External: map[string]any{
				"task": map[string]any{
					"items":    itemsAny, // []any
					"defaults": map[string]any{"mode": mode},
				},
			},
		}
		// ✅ call consumer directly
		if err := HandleSyncIbocTask(ctx, evt); err != nil {
			return fmt.Errorf("HandleSyncIbocTask (by personKey) failed: %w", err)
		}
		itemsAny = itemsAny[:0]
		chunks++
		return nil
	}

	for _, r := range rows {
		idHex := toHexID(r.ID)
		idc := strings.TrimSpace(r.IdCard)
		if idHex == "" || idc == "" {
			continue
		}
		m := map[string]any{"id": idHex, "idcard": idc}
		itemsAny = append(itemsAny, m) // append เป็น any
		if len(itemsAny) >= emitChunkSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if len(itemsAny) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}

	log.Info().Int("chunks", chunks).Msg("iboc direct-processed (by personKey)")
	return nil
}

// -------------------- Sync by personKeys (dev) --------------------
func EmitSyncIbocDevTaskByIDs(ctx context.Context, jobID, mode string, personKeys []string) error {
	tr := otel.Tracer("klynx/kwatch")
	ctx, span := tr.Start(ctx, "kwatsvc.EmitSyncIbocDevTaskByPersonKeys", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	traceID := traceutil.TraceIDFromCtx(ctx)
	log := logger.FromCtx(ctx, "kwatsvc", "EmitSyncIbocDevTaskByPersonKeys").With().
		Str("traceID", traceID).Str("jobID", jobID).Int("personKeys", len(personKeys)).Logger()

	keys := compactKeys(personKeys)
	if len(keys) == 0 {
		NotifyIbocProgressEmpty(jobID)
		log.Info().Msg("no valid personKeys")
		return nil
	}

	filter := bson.M{
		"personKey": bson.M{"$in": keys},
		"isDeleted": bson.M{"$ne": true},
		"state":     bson.M{"$ne": "deleted"},
	}
	proj := bson.M{"_id": 1, "idcard": 1}
	opt := options.Find().SetProjection(proj).SetBatchSize(2000)

	var rows []struct {
		ID     any    `bson:"_id"`
		IdCard string `bson:"idcard"`
	}
	if err := stomongo.Find(ctx, watchlistColl, filter, opt, &rows); err != nil {
		return fmt.Errorf("mongo find by personKey(dev): %w", err)
	}
	if len(rows) == 0 {
		NotifyIbocProgressEmpty(jobID)
		log.Info().Msg("no candidates (by personKey, dev)")
		return nil
	}

	now := time.Now().UTC()
	if mode == "" {
		mode = "byids"
	}
	chunks := 0

	itemsAny := make([]any, 0, emitChunkSize)

	flush := func() error {
		if len(itemsAny) == 0 {
			return nil
		}
		evt := kwatmod.WatchlistEvent{
			ID:      jobID,
			Event:   "watchlist.ibocdev.sync.task",
			TraceID: traceID,
			Time:    now.Format(time.RFC3339Nano),
			External: map[string]any{
				"task": map[string]any{
					"items":    itemsAny, // []any
					"defaults": map[string]any{"mode": mode},
				},
			},
		}
		// ✅ call consumer directly (DEV)
		if err := HandleSyncIbocDevTask(ctx, evt); err != nil {
			return fmt.Errorf("HandleSyncIbocDevTask (by personKey) failed: %w", err)
		}
		itemsAny = itemsAny[:0]
		chunks++
		return nil
	}

	for _, r := range rows {
		idHex := toHexID(r.ID)
		idc := strings.TrimSpace(r.IdCard)
		if idHex == "" || idc == "" {
			continue
		}
		m := map[string]any{"id": idHex, "idcard": idc}
		itemsAny = append(itemsAny, m)
		if len(itemsAny) >= emitChunkSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if len(itemsAny) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}

	log.Info().Int("chunks", chunks).Msg("ibocdev direct-processed (by personKey)")
	return nil
}

func compactKeys(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
