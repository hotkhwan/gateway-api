// internal/repo/cacherejected/rejected.go
package cacherejected

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	prefix   = "rpp:rejected:"
	cacheTTL = 1 * time.Hour
)

func key(orgId, sourceFamily, fingerprint string) string {
	return fmt.Sprintf("%s%s:%s:%s", prefix, orgId, sourceFamily, fingerprint)
}

// IsRejected checks whether this fingerprint has been rejected (Redis cache).
func IsRejected(ctx context.Context, orgId, sourceFamily, fingerprint string) bool {
	if config.Redis == nil {
		return false
	}
	exists, err := config.Redis.Exists(ctx, key(orgId, sourceFamily, fingerprint)).Result()
	return err == nil && exists > 0
}

// SetRejected marks a fingerprint as rejected in cache.
func SetRejected(ctx context.Context, orgId, sourceFamily, fingerprint string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Set(ctx, key(orgId, sourceFamily, fingerprint), "1", cacheTTL).Err()
}

// ClearRejected removes the rejected cache entry (e.g., when pattern is deleted).
func ClearRejected(ctx context.Context, orgId, sourceFamily, fingerprint string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx, key(orgId, sourceFamily, fingerprint)).Err()
}
