package crimes

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/kafka/kwatchpub"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SyncCrimesToWatchlist:
// - Upsert แบบ last-wins ต่อ personKey ด้วย Aggregation Pipeline
// - Revive ถ้าเคยถูก soft-delete (isDeleted=false, state=active, ลบ deletedAt ด้วย $$REMOVE)
// - Sweep แบบ soft-delete + publish delete event
func SyncCrimesToWatchlist(
	ctx context.Context,
	rows []models.CrimeImport,
	_ []string, _ []string,
	coll *mongo.Collection,
) (*models.ImportSummary, error) {

	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/crimes", "crimes.SyncCrimesToWatchlist", "crimes", "SyncCrimesToWatchlist")
	defer end()

	emitUpsert := utils.GetenvBool("KWATCH_EMIT_UPSERT", false)
	batchSize := utils.GetenvInt("KWATCH_BATCHSIZE", 2000)

	runAt := time.Now().UTC()
	runID := runAt.Format("20060102T150405Z")
	now := time.Now().UTC()

	summary := &models.ImportSummary{}
	opsByKey := make(map[string]*mongo.UpdateOneModel, batchSize)
	maybeSet := make(map[string]struct{}, batchSize)

	keysFromSet := func(m map[string]struct{}) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	chunk := func(in []string, n int) [][]string {
		if n <= 0 {
			n = 1000
		}
		out := make([][]string, 0, (len(in)+n-1)/n)
		for i := 0; i < len(in); i += n {
			j := i + n
			if j > len(in) {
				j = len(in)
			}
			out = append(out, in[i:j])
		}
		return out
	}

	flush := func(ctx context.Context) error {
		if len(opsByKey) > 0 {
			models := make([]mongo.WriteModel, 0, len(opsByKey))
			for _, m := range opsByKey {
				models = append(models, m)
			}
			c, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			res, err := coll.BulkWrite(c, models, options.BulkWrite().SetOrdered(false).SetBypassDocumentValidation(true))
			if err != nil {
				log.Error().Err(err).Int("ops", len(models)).Msg("sync to mongodb failed")
				return err
			}
			summary.Inserted += res.UpsertedCount
			summary.Updated += res.ModifiedCount
		}

		if emitUpsert {
			candidates := keysFromSet(maybeSet)
			if len(candidates) > 0 {
				changed := make([]string, 0, len(candidates))
				for _, ks := range chunk(candidates, 1000) {
					cur, err := coll.Find(ctx, bson.M{
						"source":      "crimes",
						"personKey":   bson.M{"$in": ks},
						"changeRunId": runID,
					}, options.Find().SetProjection(bson.M{"personKey": 1}))
					if err != nil {
						continue
					}
					for cur.Next(ctx) {
						var d struct {
							PersonKey string `bson:"personKey"`
						}
						if err := cur.Decode(&d); err == nil && d.PersonKey != "" {
							changed = append(changed, d.PersonKey)
						}
					}
					_ = cur.Close(ctx)
				}
				if len(changed) > 0 {
					kwatchpub.PublishUpsertBatches(ctx, "crimes", runID, changed)
				}
			}
		}

		opsByKey = make(map[string]*mongo.UpdateOneModel, batchSize)
		maybeSet = make(map[string]struct{}, batchSize)
		return nil
	}

	for i, row := range rows {
		pk := strings.TrimSpace(row.PersonKey)
		if pk == "" {
			continue
		}
		alert := buildAlertDesc(row.Warrants)
		station := ""
		if len(row.Warrants) > 0 {
			station = row.Warrants[0].PoliceStation
		}
		fp := makeFingerprint(row)

		filter := bson.M{"personKey": pk, "source": "crimes"}

		// ✅ ใช้ $$REMOVE เพื่อลบ deletedAt แบบมีเงื่อนไข (ห้ามใช้ $unset + $cond)
		update := mongo.Pipeline{
			bson.D{{Key: "$set", Value: bson.M{
				// fixed fields
				"personKey":          pk,
				"idcard":             row.IdNo,
				"type":               2,
				"crimesType":         1,
				"alertType":          "NOTIALART",
				"alertDesc":          alert,
				"passport":           row.PassportNo,
				"fullName":           row.FullName,
				"titlename":          row.TitleString,
				"firstname":          row.FirstName,
				"lastname":           row.LastName,
				"titleEn":            row.TitleEn,
				"firstNameEn":        row.FirstNameEn,
				"middleNameEn":       row.MiddleNameEn,
				"lastNameEn":         row.LastNameEn,
				"sex":                row.Sex,
				"birthday":           row.Birthday,
				"nation":             row.Nation,
				"country":            row.Country,
				"alertPoliceStation": station,
				"warrants":           row.Warrants,

				// control
				"seenAt":      runAt,
				"updatedAt":   bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{"$fingerprint", fp}}, now, "$updatedAt"}},
				"changeRunId": bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{"$fingerprint", fp}}, runID, "$changeRunId"}},
				"fingerprint": fp,

				// revive flags
				"isDeleted": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$isDeleted", true}}, false, "$isDeleted"}},
				"state":     bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$isDeleted", true}}, "active", "$state"}},
				// ถ้าเคยลบ -> ลบ deletedAt โดยตั้งเป็น $$REMOVE
				"deletedAt": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$isDeleted", true}}, "$$REMOVE", "$deletedAt"}},

				// set-on-insert
				"createdAt": bson.M{"$ifNull": bson.A{"$createdAt", now}},
				"source":    bson.M{"$ifNull": bson.A{"$source", "crimes"}},
			}}},
		}

		opsByKey[pk] = mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true).
			SetHint("uniq_crimes_person") // ปรับตามชื่อ index จริงของคุณ
		maybeSet[pk] = struct{}{}

		if len(opsByKey) >= batchSize || i == len(rows)-1 {
			if err := flush(ctx); err != nil {
				return summary, err
			}
		}
	}

	// ---------- Sweep: Soft-Delete ----------
	delFilter := bson.M{
		"source":    "crimes",
		"seenAt":    bson.M{"$lt": runAt},
		"isDeleted": bson.M{"$ne": true},
	}

	type delDoc struct {
		PersonKey      string `bson:"personKey"`
		IDCard         string `bson:"idcard"`
		PhotoOriginKey string `bson:"photoOriginKey"`
		PhotoFaceKey   string `bson:"photoFaceKey"`
		PhotoKeyLegacy string `bson:"photoKey"`
		External       struct {
			IBOC     struct{ ID, FaceID string } `bson:"iboc"`
			IBOCDev  struct{ ID, FaceID string } `bson:"ibocdev"`
			Watchman struct {
				ID     string `bson:"id"`
				IDCard string `bson:"idCard"`
			} `bson:"watchman"`
		} `bson:"external"`
	}

	cur, err := coll.Find(ctx, delFilter, options.Find().SetProjection(bson.M{
		"personKey": 1, "idcard": 1, "photoOriginKey": 1, "photoFaceKey": 1, "photoKey": 1, "external": 1,
	}))
	if err != nil {
		return summary, err
	}
	var toArchive []delDoc
	if err := cur.All(ctx, &toArchive); err != nil {
		return summary, err
	}

	ures, err := stomongo.SoftDeleteMany(ctx, coll.Name(), delFilter, stomongo.SoftDeleteOptions{
		State:              "archived",
		ClearPhotos:        true,
		ExternalNamespaces: []string{"iboc", "ibocdev", "watchman"},
		ExtraSet:           bson.M{"deletedReason": "not_seen_in_run", "runId": runID},
	})
	if err != nil {
		return summary, err
	}
	summary.Deleted = ures.ModifiedCount

	for _, d := range toArchive {
		kwatchpub.PublishDelete(ctx, "crimes", kwatchpub.DeletePayload{
			PersonKey:      d.PersonKey,
			IDCard:         d.IDCard,
			PhotoOriginKey: d.PhotoOriginKey,
			PhotoFaceKey:   d.PhotoFaceKey,
			PhotoKeyLegacy: d.PhotoKeyLegacy,
			IBOC:           kwatchpub.IBOCRef{ID: d.External.IBOC.ID, FaceID: d.External.IBOC.FaceID},
			IBOCDev:        kwatchpub.IBOCRef{ID: d.External.IBOCDev.ID, FaceID: d.External.IBOCDev.FaceID},
			External: map[string]any{
				"iboc":     map[string]any{"id": d.External.IBOC.ID, "faceId": d.External.IBOC.FaceID},
				"ibocdev":  map[string]any{"id": d.External.IBOCDev.ID, "faceId": d.External.IBOCDev.FaceID},
				"watchman": map[string]any{"id": d.External.Watchman.ID, "idCard": d.External.Watchman.IDCard},
			},
			Rev:   revFromRun(runID),
			RunID: runID,
			At:    time.Now().UTC(),
		})
	}

	log.Info().
		Int64("inserted", summary.Inserted).
		Int64("updated", summary.Updated).
		Int64("softDeleted", summary.Deleted).
		Msg("🗃️ MongoDB write summary")

	return summary, nil
}

func makeFingerprint(row models.CrimeImport) string {
	var b strings.Builder
	b.WriteString(row.IdNo)
	b.WriteString("|")
	b.WriteString(row.PassportNo)
	b.WriteString("|")
	b.WriteString(row.FullName)
	b.WriteString("|")
	b.WriteString(row.Sex)
	b.WriteString("|")
	if row.Birthday != nil {
		b.WriteString(row.Birthday.UTC().Format(time.RFC3339))
	}
	b.WriteString("|")
	for _, w := range row.Warrants {
		b.WriteString(w.WarrantNo)
		b.WriteString("/")
		b.WriteString(w.WarrantYear)
		b.WriteString("/")
		b.WriteString(w.Charge)
		b.WriteString("/")
		b.WriteString(w.PoliceRegion)
		b.WriteString("/")
		b.WriteString(w.PoliceProvincial)
		b.WriteString("/")
		b.WriteString(w.PoliceStation)
		b.WriteString("|")
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func revFromRun(runID string) int64 {
	const layout = "20060102T150405Z"
	if t, err := time.Parse(layout, runID); err == nil {
		return t.Unix()
	}
	return time.Now().Unix()
}
