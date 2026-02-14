// internal/services/authzsvc/bulkwriter.go
package authzsvc

import (
	"context"
	"time"
)

// BulkWriteTuples performs batched writes/deletes. This is a placeholder that simulates work.
func BulkWriteTuples(ctx context.Context, ops []interface{}) error {
	// TODO: implement retry/backoff and actual calls to Permify or DB
	time.Sleep(10 * time.Millisecond)
	return nil
}
