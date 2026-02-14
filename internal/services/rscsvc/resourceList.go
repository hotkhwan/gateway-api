// internal/services/rscsvc/resourceList.go
package rscsvc

import (
	"context"
	"errors"
	"strings"
	"time"

	"klynx/internal/repo/stomongo"
	"klynx/models/gmod"
	"klynx/models/rscmod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var allowedSortFields = map[string]struct{}{
	"updatedAt":   {},
	"createdAt":   {},
	"displayName": {},
}
var ErrDuplicateCanonicalID = errors.New("duplicate canonicalId")

func sanitizeSortField(in string) string {
	in = strings.TrimSpace(in)
	if _, ok := allowedSortFields[in]; ok {
		return in
	}
	return "updatedAt"
}

func ResourceList(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]rscmod.Resource, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/rscsvc", // tracer name
		"ResourceList", // span name
		"rscsvc",       // service tag
		"ResourceList", // operation tag
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	filter := bson.M{
		"deletedAt": bson.M{"$exists": false},
	}
	if v := filters["provider"]; v != "" {
		filter["provider"] = v
	}
	if v := filters["type"]; v != "" {
		filter["type"] = v
	}
	if v := filters["search"]; v != "" {
		filter["displayName"] = bson.M{"$regex": v, "$options": "i"}
	}

	if page <= 0 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}
	skip := (page - 1) * perPages

	sf := sanitizeSortField(sortField)
	dir := -1
	if strings.ToLower(sortOrder) == "asc" {
		dir = 1
	}

	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sf, Value: dir}, {Key: "_id", Value: dir}})

	var rows []rscmod.Resource
	if err := stomongo.Find(ctx, "resources", filter, opts, &rows); err != nil {
		log.Error().Err(err).Msg("❌ Mongo Find failed")
		return nil, gmod.Pagination{}, err
	}

	total64, err := stomongo.Count(ctx, "resources", filter)
	if err != nil {
		return nil, gmod.Pagination{}, err
	}
	total := int(total64)

	return rows, gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: total,
		TotalPages:   (total + perPages - 1) / perPages,
		SortField:    sf,
		SortOrder:    strings.ToLower(sortOrder),
	}, nil
}

func ResourceCreate(ctx context.Context, doc *rscmod.Resource) (string, error) {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"klynx/rscsvc",
		"ResourceCreate",
		"rscsvc",
		"ResourceCreate",
	)
	defer end()

	// 1) เช็คซ้ำในแอป (เฉพาะที่ยังไม่ถูกลบ)
	dupFilter := bson.M{
		"canonicalId": doc.CanonicalId,
		"deletedAt":   bson.M{"$exists": false},
	}
	exists, err := stomongo.Count(ctx, "resources", dupFilter)
	if err != nil {
		return "", err
	}
	if exists > 0 {
		return "", ErrDuplicateCanonicalID
	}

	// 2) ตั้ง timestamp
	now := time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	doc.UpdatedAt = now

	// 3) Insert
	oid, err := stomongo.InsertOne(ctx, "resources", doc)
	if err != nil {
		// กัน race condition ถ้ามี unique index canonicalId
		if mongo.IsDuplicateKeyError(err) {
			return "", ErrDuplicateCanonicalID
		}
		return "", err
	}
	return oid.Hex(), nil
}

func ResourceUpdate(ctx context.Context, id string, doc *rscmod.Resource) error {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"klynx/rscsvc",
		"ResourceUpdate",
		"rscsvc",
		"ResourceUpdate",
	)
	defer end()

	// หาได้ทั้ง canonicalId และ id ภายใน
	filter := bson.M{
		"deletedAt": bson.M{"$exists": false},
		"$or": []bson.M{
			{"canonicalId": id},
			{"id": id},
		},
	}

	// บังคับอัปเดต updatedAt
	doc.UpdatedAt = time.Now()

	update := bson.M{"$set": doc}
	res, err := stomongo.UpdateOne(ctx, "resources", filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.New("resource not found")
	}
	return nil
}

func ResourceDelete(ctx context.Context, id string) error {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"klynx/rscsvc",
		"ResourceDelete",
		"rscsvc",
		"ResourceDelete",
	)
	defer end()

	now := time.Now()
	filter := bson.M{
		"deletedAt": bson.M{"$exists": false},
		"$or": []bson.M{
			{"canonicalId": id},
			{"id": id},
		},
	}
	update := bson.M{"$set": bson.M{"deletedAt": now, "updatedAt": now}}
	_, err := stomongo.UpdateOne(ctx, "resources", filter, update)
	return err
}
