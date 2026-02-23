// internal/adapters/repo/stomongo/findPaginated.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PaginateOptions struct {
	Page    int
	PerPage int
	Sort    bson.D // e.g. bson.D{{Key: "createdAt", Value: -1}}
}

// FindPaginated — Find พร้อม skip/limit/sort สำหรับ pagination
// out ต้องเป็น pointer to slice เช่น &[]MyModel{}
func FindPaginated(ctx context.Context, collection string, filter interface{}, pag PaginateOptions, out any) error {
	if pag.Page < 1 {
		pag.Page = 1
	}
	if pag.PerPage <= 0 {
		pag.PerPage = 10
	}

	opts := options.Find().
		SetSkip(int64((pag.Page - 1) * pag.PerPage)).
		SetLimit(int64(pag.PerPage))

	if len(pag.Sort) > 0 {
		opts.SetSort(pag.Sort)
	}

	return Find(ctx, collection, filter, opts, out)
}
