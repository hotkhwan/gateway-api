// internal/adapters/repo/stomongo/count.go
package stomongo

import "context"

func Count(ctx context.Context, collection string, filter interface{}) (int64, error) {
	return coll(collection).CountDocuments(ctx, filter)
}
