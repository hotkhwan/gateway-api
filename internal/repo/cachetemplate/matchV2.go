// internal/repo/cachetemplate/matchV2.go
package cachetemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	matchV2Prefix       = "tmpl:match:"
	matchV2CacheTTL     = 6 * time.Hour
	matchV2NegativeTTL  = 30 * time.Second
	matchV2NoMatchValue = "__none__"
)

// MatchV2CacheValue is stored in Redis for a matched template.
type MatchV2CacheValue struct {
	TemplateId     string `json:"templateId"`
	FinalEventType string `json:"finalEventType,omitempty"`
	SourceFamily   string `json:"sourceFamily"`
	Fingerprint    string `json:"fingerprint"`
}

func matchV2Key(tenantId, orgId, sourceFamily, fingerprint string) string {
	return fmt.Sprintf("%s%s:%s:%s:%s", matchV2Prefix, tenantId, orgId, sourceFamily, fingerprint)
}

// GetMatchV2 returns a cached template match or nil on miss.
// Returns (value, true) on hit, (nil, true) on confirmed no-match, (nil, false) on miss.
func GetMatchV2(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint string) (*MatchV2CacheValue, bool) {
	if config.Redis == nil {
		return nil, false
	}
	val, err := config.Redis.Get(ctx, matchV2Key(tenantId, orgId, sourceFamily, fingerprint)).Bytes()
	if err != nil {
		return nil, false
	}
	if string(val) == matchV2NoMatchValue {
		return nil, true
	}
	var v MatchV2CacheValue
	if json.Unmarshal(val, &v) != nil {
		return nil, false
	}
	return &v, true
}

// SetMatchV2 caches a template match result.
func SetMatchV2(ctx context.Context, tenantId, orgId string, v *MatchV2CacheValue) {
	if config.Redis == nil || v == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = config.Redis.Set(ctx, matchV2Key(tenantId, orgId, v.SourceFamily, v.Fingerprint), b, matchV2CacheTTL).Err()
}

// SetNoMatchV2 caches a confirmed no-match for a fingerprint.
func SetNoMatchV2(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Set(ctx, matchV2Key(tenantId, orgId, sourceFamily, fingerprint), matchV2NoMatchValue, matchV2NegativeTTL).Err()
}

// InvalidateMatchV2ByFamily removes all V2 match cache for a source family (uses SCAN).
func InvalidateMatchV2ByFamily(ctx context.Context, tenantId, orgId, sourceFamily string) {
	if config.Redis == nil {
		return
	}
	pattern := fmt.Sprintf("%s%s:%s:%s:*", matchV2Prefix, tenantId, orgId, sourceFamily)
	iter := config.Redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		_ = config.Redis.Del(ctx, keys...).Err()
	}
}
