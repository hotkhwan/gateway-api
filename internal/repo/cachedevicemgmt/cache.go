// internal/repo/cachedevicemgmt/cache.go
package cachedevicemgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

const (
	prefix   = "device:mgmt:"
	cacheTTL = 30 * time.Minute
)

func key(tenantId, workspaceId, sourceFamily, entityType, entityId string) string {
	return fmt.Sprintf("%s%s:%s:%s:%s:%s", prefix, tenantId, workspaceId, sourceFamily, entityType, entityId)
}

// Get returns cached device management record or nil on miss.
func Get(ctx context.Context, tenantId, workspaceId, sourceFamily, entityType, entityId string) *ingestmod.DeviceManagement {
	if config.Redis == nil {
		return nil
	}
	val, err := config.Redis.Get(ctx, key(tenantId, workspaceId, sourceFamily, entityType, entityId)).Bytes()
	if err != nil {
		return nil
	}
	var d ingestmod.DeviceManagement
	if json.Unmarshal(val, &d) != nil {
		return nil
	}
	return &d
}

// Set caches a device management record.
func Set(ctx context.Context, d *ingestmod.DeviceManagement) {
	if config.Redis == nil || d == nil {
		return
	}
	b, err := json.Marshal(d)
	if err != nil {
		return
	}
	_ = config.Redis.Set(ctx, key(d.TenantId, d.WorkspaceId, d.SourceFamily, d.EntityType, d.EntityId), b, cacheTTL).Err()
}

// Invalidate removes a cached device management record.
func Invalidate(ctx context.Context, tenantId, workspaceId, sourceFamily, entityType, entityId string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx, key(tenantId, workspaceId, sourceFamily, entityType, entityId)).Err()
}
