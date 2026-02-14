// internal/services/authzsvc/idempotency.go
package authzsvc

import (
	"context"
	"sync"
	"time"
)

// Simple in-memory idempotency store. Not suitable for production but useful as a placeholder.
type idempotencyRecord struct {
	Key       string
	Response  interface{}
	CreatedAt time.Time
}

var (
	idempMu  sync.RWMutex
	idempMap = map[string]idempotencyRecord{}
)

// GetIdempotency returns stored response if exists
func GetIdempotency(ctx context.Context, key string) (interface{}, bool) {
	idempMu.RLock()
	defer idempMu.RUnlock()
	r, ok := idempMap[key]
	if !ok {
		return nil, false
	}
	return r.Response, true
}

// PutIdempotency stores a response under the key
func PutIdempotency(ctx context.Context, key string, resp interface{}) {
	idempMu.Lock()
	defer idempMu.Unlock()
	idempMap[key] = idempotencyRecord{Key: key, Response: resp, CreatedAt: time.Now()}
}
