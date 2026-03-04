package cacheevt

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	statusPrefix        = "evt:status:"
	deviceApprovalPrefix = "evt:device:approved:"
	pendingTTL         = 30 * time.Minute
	approvedTTL        = 24 * time.Hour
	rejectedTTL        = 24 * time.Hour
	deviceApprovedTTL   = 7 * 24 * time.Hour // 7 days for device:eventType approval
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

// ============================================================
// Device:EventType Approval Cache
// ============================================================

// DeviceApprovalKey creates a cache key for device:eventType approval
// Format: "evt:device:approved:<tenantId>:<deviceKey>:<eventType>"
// Example: "evt:device:approved:aisom:camera:cam-001:LPR_Brand"
func DeviceApprovalKey(tenantId, deviceKey, eventType string) string {
	return fmt.Sprintf("%s%s:%s:%s", deviceApprovalPrefix, tenantId, deviceKey, eventType)
}

// IsDeviceEventTypeApproved checks if a device:eventType is approved
// Returns true if the combination exists in cache
func IsDeviceEventTypeApproved(ctx context.Context, tenantId, deviceKey, eventType string) bool {
	if config.Redis == nil {
		return false
	}

	key := DeviceApprovalKey(tenantId, deviceKey, eventType)
	exists, err := config.Redis.Exists(ctx, key).Result()
	return err == nil && exists > 0
}

// SetDeviceEventTypeApproved marks a device:eventType as approved
// This cache lasts for 7 days, after which the user must approve again
func SetDeviceEventTypeApproved(ctx context.Context, tenantId, deviceKey, eventType string) error {
	if config.Redis == nil {
		return nil
	}

	key := DeviceApprovalKey(tenantId, deviceKey, eventType)
	return config.Redis.Set(ctx, key, "1", deviceApprovedTTL).Err()
}

// InvalidateDeviceEventTypeApproval removes approval for a device:eventType
// Called when user changes approval status or when a rule is updated
func InvalidateDeviceEventTypeApproval(ctx context.Context, tenantId, deviceKey, eventType string) error {
	if config.Redis == nil {
		return nil
	}

	key := DeviceApprovalKey(tenantId, deviceKey, eventType)
	return config.Redis.Del(ctx, key).Err()
}

// InvalidateAllDeviceApprovalsForOrg removes all device approvals for an org
// Called when org is deleted or settings are reset
func InvalidateAllDeviceApprovalsForOrg(ctx context.Context, tenantId, orgId string) error {
	if config.Redis == nil {
		return nil
	}

	// Get all keys matching the pattern
	pattern := fmt.Sprintf("%s%s:*", deviceApprovalPrefix, tenantId)
	iter := config.Redis.Scan(ctx, 0, pattern, 0).Iterator()

	keysToDelete := []string{}
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if len(keysToDelete) > 0 {
		return config.Redis.Del(ctx, keysToDelete...).Err()
	}

	return nil
}
