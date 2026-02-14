// internal/adapters/repo/stomongo/findOneOpts.go
package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ใช้เมื่ออยากกำหนด projection, hint, readPreference ฯลฯ
func FindOneWithOptions(ctx context.Context, collection string, filter bson.M, out any, opts ...*options.FindOneOptions) error {
	return coll(collection).FindOne(ctx, filter, opts...).Decode(out)
}

// sugar สำหรับ projection ทั่วไป
func FindOneProjection(ctx context.Context, collection string, filter bson.M, projection bson.M, out any) error {
	return FindOneWithOptions(ctx, collection, filter, out, options.FindOne().SetProjection(projection))
}
