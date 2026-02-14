// internal/adapters/repo/stomongo/find.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Find ทำงานเหมือน collection.Find แล้ว decode ทั้งหมดลงใน out (ต้องส่ง pointer to slice)
func Find(ctx context.Context, collection string, filter interface{}, opts *options.FindOptions, out any) error {
	cur, err := coll(collection).Find(ctx, filter, opts)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	return cur.All(ctx, out)
}
