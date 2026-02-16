package rscsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/rscmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BulkCreateResult struct {
	IDs      []string
	Inserted int
	Skipped  int
	Errors   []struct {
		Index   int
		Reason  string
		Details string
	}
}

func makeKey(provider, typ, canonicalId string) string {
	return provider + "|" + typ + "|" + canonicalId
}

func ResourceBulkCreate(ctx context.Context, payloads []rscmod.ResourceUpsert) (BulkCreateResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx, "github.com/hotkhwan/gateway-api/rscsvc", "ResourceBulkCreate", "rscsvc", "ResourceBulkCreate",
	)
	defer end()

	res := BulkCreateResult{
		IDs: []string{},
		Errors: []struct {
			Index   int
			Reason  string
			Details string
		}{},
	}

	now := time.Now()

	// 1) ตรวจสอบและตัด duplicate ใน payload เดียวกัน
	seenInBatch := make(map[string]int) // key -> first index
	validPayloadIdx := make([]int, 0, len(payloads))

	for i, p := range payloads {
		if p.CanonicalId == "" || p.Provider == "" || p.Type == "" || p.DisplayName == "" {
			res.Errors = append(res.Errors, struct {
				Index   int
				Reason  string
				Details string
			}{
				Index:   i,
				Reason:  "validation",
				Details: "canonicalId, provider, type, displayName are required",
			})
			continue
		}
		k := makeKey(p.Provider, p.Type, p.CanonicalId)
		if first, dup := seenInBatch[k]; dup {
			res.Errors = append(res.Errors, struct {
				Index   int
				Reason  string
				Details string
			}{
				Index:   i,
				Reason:  "duplicate_in_payload",
				Details: fmt.Sprintf("same canonicalId+provider+type as index %d", first),
			})
			continue
		}
		seenInBatch[k] = i
		validPayloadIdx = append(validPayloadIdx, i)
	}

	if len(validPayloadIdx) == 0 {
		res.Skipped = len(payloads)
		return res, nil
	}

	// 2) ตรวจสอบซ้ำกับ DB (existing)
	type triple struct{ Provider, Type, CanonicalId string }
	triples := make([]triple, 0, len(validPayloadIdx))
	for _, idx := range validPayloadIdx {
		p := payloads[idx]
		triples = append(triples, triple{Provider: p.Provider, Type: p.Type, CanonicalId: p.CanonicalId})
	}

	const chunkSize = 500
	existingKeys := make(map[string]struct{})

	for start := 0; start < len(triples); start += chunkSize {
		endIdx := start + chunkSize
		if endIdx > len(triples) {
			endIdx = len(triples)
		}
		orConds := make([]bson.M, 0, endIdx-start)
		for _, t := range triples[start:endIdx] {
			orConds = append(orConds, bson.M{
				"provider":    t.Provider,
				"type":        t.Type,
				"canonicalId": t.CanonicalId,
			})
		}
		filter := bson.M{"$or": orConds}

		// ✅ ใช้ stomongo.Find ที่คืนค่าเป็น error เดียว และต้องส่ง out slice
		var found []struct {
			Provider    string `bson:"provider"`
			Type        string `bson:"type"`
			CanonicalId string `bson:"canonicalId"`
		}
		findOpts := options.Find().SetProjection(bson.M{
			"provider":    1,
			"type":        1,
			"canonicalId": 1,
			"_id":         0,
		})
		if err := stomongo.Find(ctx, "resources", filter, findOpts, &found); err != nil {
			log.Error().Err(err).Msg("query existing resources failed")
			return res, err
		}
		for _, doc := range found {
			existingKeys[makeKey(doc.Provider, doc.Type, doc.CanonicalId)] = struct{}{}
		}
	}

	// 3) เตรียม docs สำหรับ insert (ข้ามที่พบว่าซ้ำใน DB)
	docs := make([]interface{}, 0, len(validPayloadIdx))
	for _, idx := range validPayloadIdx {
		p := payloads[idx]
		k := makeKey(p.Provider, p.Type, p.CanonicalId)
		if _, exists := existingKeys[k]; exists {
			res.Errors = append(res.Errors, struct {
				Index   int
				Reason  string
				Details string
			}{
				Index:   idx,
				Reason:  "duplicate_existing",
				Details: "already exists (same canonicalId+provider+type)",
			})
			continue
		}

		docs = append(docs, rscmod.Resource{
			CanonicalId: p.CanonicalId,
			Provider:    p.Provider,
			Type:        p.Type,
			Id:          p.Id,
			ExternalId:  p.ExternalId,
			DisplayName: p.DisplayName,
			Icon:        p.Icon,
			Path:        p.Path,
			Tags:        p.Tags,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	if len(docs) == 0 {
		res.Skipped = len(payloads) - res.Inserted
		return res, nil
	}

	// 4) InsertMany (proxy)
	ids, merr := stomongo.InsertMany(ctx, "resources", docs, nil)

	for _, oid := range ids {
		if oid.IsZero() {
			continue
		}
		res.Inserted++
		res.IDs = append(res.IDs, oid.Hex())
	}

	// รวม skipped ทั้งหมดเป็น: จำนวน payload ทั้งหมด - ที่ insert ได้จริง
	res.Skipped = len(payloads) - res.Inserted

	if merr != nil {
		log.Warn().Err(merr).Msg("bulk insert completed with partial errors")
	}

	return res, nil
}
