// internal/repo/stomongo/insertMany.go
package stomongo

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MultiError struct {
	Errors []error
}

func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return ""
	}
	msgs := make([]string, 0, len(m.Errors))
	for _, e := range m.Errors {
		if e != nil {
			msgs = append(msgs, e.Error())
		}
	}
	return strings.Join(msgs, "; ")
}

// Proxy InsertMany ผ่าน InsertOne (unordered)
// - ตำแหน่งที่ล้มเหลวจะคืน NilObjectID เพื่อให้ index ตรงกับ docs
func InsertMany(ctx context.Context, coll string, docs []interface{}, _ *options.InsertManyOptions) ([]primitive.ObjectID, error) {
	ids := make([]primitive.ObjectID, 0, len(docs))
	var merr MultiError

	for _, d := range docs {
		oid, err := InsertOne(ctx, coll, d)
		if err != nil {
			// เก็บ NilObjectID ไว้ที่ตำแหน่งที่พัง เพื่อคงลำดับให้ตรงกับ docs
			ids = append(ids, primitive.NilObjectID)
			// ✅ เก็บ error ครั้งเดียว ไม่ต้องเช็คซ้ำ
			merr.Errors = append(merr.Errors, err)
			continue
		}
		ids = append(ids, oid)
	}

	if len(merr.Errors) > 0 {
		return ids, &merr
	}
	return ids, nil
}
