// internal/repo/stomongo/pagination.go
package stomongo

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// FindWithPagination ค้นหาข้อมูลจาก collection พร้อม pagination + sort
func FindWithPagination[T any](
	ctx context.Context,
	collName string,
	filter interface{},
	sort interface{},
	page, perPage int,
	result *[]T,
) (gmod.Pagination, error) {
	coll := config.DB.Collection(collName)

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 10
	}

	skip := int64((page - 1) * perPage)
	limit := int64(perPage)

	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(limit)

	if sort != nil {
		findOpts.SetSort(sort)
	}

	cur, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return gmod.Pagination{}, err
	}
	defer cur.Close(ctx)

	if err := cur.All(ctx, result); err != nil {
		return gmod.Pagination{}, err
	}

	// นับจำนวนทั้งหมด
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return gmod.Pagination{}, err
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	return gmod.Pagination{
		Page:         page,
		PerPages:     perPage,
		TotalRecords: int(total),
		TotalPages:   totalPages,
		SortField:    "", // สามารถเติมจากพารามิเตอร์ service
		SortOrder:    "", // เช่น "asc"/"desc"
	}, nil
}
