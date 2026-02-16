// internal/repo/cachego/cacheklive/peerip.go
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
	klivePeerIPStreamPrefix = "klive:peerip:stream:"
)

func peerIPKey(stream string) string { return klivePeerIPStreamPrefix + stream }

func SetKlivePeerIPByStream(ctx context.Context, stream string, peerIP string, ttl time.Duration) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	peerIP = strings.TrimSpace(peerIP)
	if stream == "" || peerIP == "" {
		return nil
	}
	return config.Redis.Set(ctx, peerIPKey(stream), peerIP, ttl).Err()
}

func GetKlivePeerIPByStream(ctx context.Context, stream string) (string, bool, error) {
	if config.Redis == nil {
		return "", false, errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return "", false, nil
	}
	val, err := config.Redis.Get(ctx, peerIPKey(stream)).Result()
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

func DeleteKlivePeerIPByStream(ctx context.Context, stream string) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil
	}
	return config.Redis.Del(ctx, peerIPKey(stream)).Err()
}
