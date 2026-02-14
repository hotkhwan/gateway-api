// internal/repo/cachego/cacheklive/subscribe.go
package cacheklive

import (
	"context"

	"klynx/config"

	"github.com/redis/go-redis/v9"
)

// SubscribeRedisExpired
// ฟัง Redis keyevent expired (ต้องเปิด notify-keyspace-events Ex)
func SubscribeRedisExpired(ctx context.Context) *redis.PubSub {
	if config.Redis == nil {
		return nil
	}

	// ⚠️ ถ้าใช้ DB อื่น ให้เปลี่ยนเลข @0
	return config.Redis.Subscribe(ctx, "__keyevent@0__:expired")
}
