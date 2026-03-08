// internal/repo/cachereview/pending.go
package cachereview

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	prefix  = "upr:pending:"
	lockTTL = 1 * time.Hour
)

func key(orgId, sourceFamily, fingerprint string) string {
	return fmt.Sprintf("%s%s:%s:%s", prefix, orgId, sourceFamily, fingerprint)
}

// IsPending checks whether a pending review already exists for this fingerprint.
func IsPending(ctx context.Context, orgId, sourceFamily, fingerprint string) bool {
	if config.Redis == nil {
		return false
	}
	exists, err := config.Redis.Exists(ctx, key(orgId, sourceFamily, fingerprint)).Result()
	return err == nil && exists > 0
}

// SetPending marks a fingerprint as having a pending review.
func SetPending(ctx context.Context, orgId, sourceFamily, fingerprint, reviewId string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Set(ctx, key(orgId, sourceFamily, fingerprint), reviewId, lockTTL).Err()
}

// ClearPending removes the pending lock for a fingerprint.
func ClearPending(ctx context.Context, orgId, sourceFamily, fingerprint string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx, key(orgId, sourceFamily, fingerprint)).Err()
}
