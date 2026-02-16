// internal/repo/cachego/cacheklive/media.go
package cacheklive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/cachemod"
)

const (
	devicePrefix = "media:device:" // media:device:<deviceId>
	lockPrefix   = "media:lock:"   // media:lock:<streamKey>
)

func HashURL(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func deviceKey(deviceId string) string { return devicePrefix + deviceId }
func lockKey(streamKey string) string  { return lockPrefix + streamKey }

// TTL แนะนำ: 10–30 นาที (แล้วแต่ policy)
// ถ้าอยาก “cache อยู่เท่า ZLM proxy อยู่” ก็ใช้ hook ล้างแทน TTL ยาวได้
func SetDeviceStream(ctx context.Context, deviceId string, v *cachemod.MediaStreamCache, ttl time.Duration) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	if deviceId == "" || v == nil {
		return errors.New("bad args")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return config.Redis.Set(ctx, deviceKey(deviceId), string(b), ttl).Err()
}

func GetDeviceStream(ctx context.Context, deviceId string) (*cachemod.MediaStreamCache, bool, error) {
	if config.Redis == nil {
		return nil, false, errors.New("redis not initialized")
	}
	if deviceId == "" {
		return nil, false, nil
	}
	s, err := config.Redis.Get(ctx, deviceKey(deviceId)).Result()
	if err != nil {
		// redis.Nil = not found
		if err.Error() == "redis: nil" {
			return nil, false, nil
		}
		return nil, false, err
	}
	var out cachemod.MediaStreamCache
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		// ถ้า parse ไม่ได้ ล้างทิ้งกันค้าง
		_ = config.Redis.Del(ctx, deviceKey(deviceId)).Err()
		return nil, false, fmt.Errorf("bad cache json: %w", err)
	}
	return &out, true, nil
}

func DeleteDeviceStream(ctx context.Context, deviceId string) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	if deviceId == "" {
		return nil
	}
	return config.Redis.Del(ctx, deviceKey(deviceId)).Err()
}

// AcquireStreamLock: ให้ “ตรงกับ error ล่าสุดของคุณ” → return 2 ค่า
// ใช้ SET NX PX
func AcquireStreamLock(ctx context.Context, streamKey string, ttl time.Duration) (bool, error) {
	if config.Redis == nil {
		return false, errors.New("redis not initialized")
	}
	if streamKey == "" {
		return false, nil
	}
	// ใช้ value คงที่ก็ได้ เพราะเราไม่ต้องคืน token
	return config.Redis.SetNX(ctx, lockKey(streamKey), "1", ttl).Result()
}

func ReleaseStreamLock(ctx context.Context, streamKey string) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	if streamKey == "" {
		return nil
	}
	return config.Redis.Del(ctx, lockKey(streamKey)).Err()
}
