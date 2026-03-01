package cacheevt

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	statusPrefix   = "evt:status:"
	pendingTTL   = 30 * time.Minute
	approvedTTL  = 24 * time.Hour
	rejectedTTL  = 24 * time.Hour
)

// StatusKey creates a cache key for event status
func StatusKey(tenantId, eventId string) string {
	return fmt.Sprintf("%s%s:%s", statusPrefix, tenantId, eventId)
}

// GetEventStatus retrieves event status from cache
// Returns (status, found)
func GetEventStatus(ctx context.Context, tenantId, eventId string) (string, bool) {
	if config.Redis == nil {
		return "", false
	}

	key := StatusKey(tenantId, eventId)
	val, err := config.Redis.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// SetEventStatusPending caches event as pending
func SetEventStatusPending(ctx context.Context, tenantId, eventId string) error {
	return SetEventStatus(ctx, tenantId, eventId, "pending", pendingTTL)
}

// SetEventStatusApproved caches event as approved
func SetEventStatusApproved(ctx context.Context, tenantId, eventId string) error {
	return SetEventStatus(ctx, tenantId, eventId, "approved", approvedTTL)
}

// SetEventStatusRejected caches event as rejected
func SetEventStatusRejected(ctx context.Context, tenantId, eventId string) error {
	return SetEventStatus(ctx, tenantId, eventId, "rejected", rejectedTTL)
}

// SetEventStatus caches event status with TTL
func SetEventStatus(ctx context.Context, tenantId, eventId, status string, ttl time.Duration) error {
	if config.Redis == nil {
		return nil
	}

	key := StatusKey(tenantId, eventId)
	return config.Redis.Set(ctx, key, status, ttl).Err()
}

// InvalidateEventStatus removes event from cache
func InvalidateEventStatus(ctx context.Context, tenantId, eventId string) error {
	if config.Redis == nil {
		return nil
	}

	key := StatusKey(tenantId, eventId)
	return config.Redis.Del(ctx, key).Err()
}
