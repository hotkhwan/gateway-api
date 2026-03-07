// internal/repo/cachereview/pending.go
package cachereview

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	prefix  = "tmpl:review:pending:"
	lockTTL = 1 * time.Hour
)

func key(tenantId, orgId, sourceFamily, fingerprint string) string {
	return fmt.Sprintf("%s%s:%s:%s:%s", prefix, tenantId, orgId, sourceFamily, fingerprint)
}

// IsPending checks whether a pending review already exists for this fingerprint.
func IsPending(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint string) bool {
	if config.Redis == nil {
		return false
	}
	exists, err := config.Redis.Exists(ctx, key(tenantId, orgId, sourceFamily, fingerprint)).Result()
	return err == nil && exists > 0
}

// SetPending marks a fingerprint as having a pending review.
func SetPending(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint, reviewId string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Set(ctx, key(tenantId, orgId, sourceFamily, fingerprint), reviewId, lockTTL).Err()
}

// ClearPending removes the pending lock for a fingerprint.
func ClearPending(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx, key(tenantId, orgId, sourceFamily, fingerprint)).Err()
}
