// internal/repo/cachesourceprofile/cache.go
package cachesourceprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

const (
	prefix   = "source:profile:"
	cacheTTL = 1 * time.Hour
)

func key(sourceFamily string) string {
	return fmt.Sprintf("%s%s", prefix, sourceFamily)
}

// Get returns cached source profile or nil on miss.
func Get(ctx context.Context, sourceFamily string) *ingestmod.SourceProfile {
	if config.Redis == nil {
		return nil
	}
	val, err := config.Redis.Get(ctx, key(sourceFamily)).Bytes()
	if err != nil {
		return nil
	}
	var p ingestmod.SourceProfile
	if json.Unmarshal(val, &p) != nil {
		return nil
	}
	return &p
}

// Set caches a source profile.
func Set(ctx context.Context, p *ingestmod.SourceProfile) {
	if config.Redis == nil || p == nil {
		return
	}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = config.Redis.Set(ctx, key(p.SourceFamily), b, cacheTTL).Err()
}

// Invalidate removes a cached source profile.
func Invalidate(ctx context.Context, sourceFamily string) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx, key(sourceFamily)).Err()
}
