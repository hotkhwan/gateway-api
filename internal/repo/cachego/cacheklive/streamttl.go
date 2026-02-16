package cacheklive

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"

	"github.com/redis/go-redis/v9"
)

const (
	kliveStreamTTLPrefix = "klive:ttl:stream:"
)

func streamTTLKey(stream string) string { return kliveStreamTTLPrefix + stream }

// SetStreamTTLKey sets a small marker key that expires and triggers Redis keyevent.
// Value is irrelevant; expiry time is the signal.
// This key SHOULD be used as the expiry trigger (watcher listens to it).
func SetStreamTTLKey(ctx context.Context, stream string, ttl time.Duration) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" || ttl <= 0 {
		return nil
	}
	return config.Redis.Set(ctx, streamTTLKey(stream), "1", ttl).Err()
}

func GetStreamTTLKey(ctx context.Context, stream string) (string, bool, error) {
	if config.Redis == nil {
		return "", false, errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return "", false, nil
	}
	val, err := config.Redis.Get(ctx, streamTTLKey(stream)).Result()
	if err != nil {
		if err == redis.Nil || err.Error() == "redis: nil" {
			return "", false, nil
		}
		return "", false, err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "", false, nil
	}
	return val, true, nil
}

func DeleteStreamTTLKey(ctx context.Context, stream string) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil
	}
	return config.Redis.Del(ctx, streamTTLKey(stream)).Err()
}
