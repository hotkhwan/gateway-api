// internal/repo/cachetemplate/fingerprint.go
package cachetemplate

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

const (
	fingerprintPrefix = "tmpl:fp:v1:"
	cacheTTL          = 5 * time.Minute  // short TTL so template updates propagate quickly
	negativeCacheTTL  = 30 * time.Second // negative cache: no template found
	noMatchSentinel   = "__none__"
)

// fingerprintKey returns the Redis key for orgId + eventType + keyHash.
func fingerprintKey(orgId, eventType, keyHash string) string {
	return fmt.Sprintf("%s%s:%s:%s", fingerprintPrefix, orgId, eventType, keyHash)
}

// GetTemplateId returns the cached templateId for the given fingerprint.
// Returns ("", false) on cache miss, (noMatchSentinel, true) on confirmed no-match.
func GetTemplateId(ctx context.Context, orgId, eventType, keyHash string) (templateId string, found bool) {
	if config.Redis == nil {
		return "", false
	}
	val, err := config.Redis.Get(ctx, fingerprintKey(orgId, eventType, keyHash)).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// SetTemplateId caches a templateId for the given fingerprint.
func SetTemplateId(ctx context.Context, orgId, eventType, keyHash, templateId string) error {
	if config.Redis == nil {
		return nil
	}
	return config.Redis.Set(ctx, fingerprintKey(orgId, eventType, keyHash), templateId, cacheTTL).Err()
}

// SetNoMatch caches a negative result (no template found) for the given fingerprint.
func SetNoMatch(ctx context.Context, orgId, eventType, keyHash string) error {
	if config.Redis == nil {
		return nil
	}
	return config.Redis.Set(ctx, fingerprintKey(orgId, eventType, keyHash), noMatchSentinel, negativeCacheTTL).Err()
}

// IsNoMatch returns true when the cached value represents a confirmed no-match.
func IsNoMatch(templateId string) bool {
	return templateId == noMatchSentinel
}

// InvalidateByOrg removes all fingerprint cache entries for an org.
// Call after template create/update/delete so the next ingest re-queries Mongo.
func InvalidateByOrg(ctx context.Context, orgId string) error {
	if config.Redis == nil {
		return nil
	}
	pattern := fmt.Sprintf("%s%s:*", fingerprintPrefix, orgId)
	iter := config.Redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return config.Redis.Del(ctx, keys...).Err()
	}
	return nil
}
