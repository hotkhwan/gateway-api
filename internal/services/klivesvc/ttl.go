// internal/services/klivesvc/ttl.go
package klivesvc

import (
	"context"
	"sync"
	"time"

	"klynx/config"

	"go.mongodb.org/mongo-driver/bson"
)

const defaultSessionTTL = 3600 // seconds (ใช้เป็น TTL ของ clientInfo ด้วย)

var (
	cachedTTL     int64
	cachedAt      time.Time
	cacheMu       sync.Mutex
	cacheDuration = 30 * time.Second
)

func GetEffectiveSessionTTL(ctx context.Context) int {
	now := time.Now()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cachedTTL > 0 && now.Sub(cachedAt) < cacheDuration {
		return int(cachedTTL)
	}

	var opt bson.M
	err := config.DB.
		Collection("options").
		FindOne(ctx, bson.M{"_id": "system.setting"}).
		Decode(&opt)

	ttl := int64(defaultSessionTTL)
	if err == nil && opt != nil {
		if v, ok := opt["streamSessionTimeout"]; ok {
			switch n := v.(type) {
			case int64:
				if n > 0 {
					ttl = n
				}
			case int32:
				if n > 0 {
					ttl = int64(n)
				}
			case int:
				if n > 0 {
					ttl = int64(n)
				}
			case float64:
				if n > 0 {
					ttl = int64(n)
				}
			}
		}
	}

	if ttl <= 0 {
		ttl = 15
	}

	cachedTTL = ttl
	cachedAt = now
	return int(ttl)
}
