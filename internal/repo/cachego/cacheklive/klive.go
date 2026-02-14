// internal/repo/cachego/cacheklive/klive.go
package cacheklive

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"klynx/config"
	"klynx/models/klivemod"
)

const (
	kliveClientInfoStreamPrefix = "klive:clientinfo:stream:"
)

func kliveStreamKey(stream string) string { return kliveClientInfoStreamPrefix + stream }

func normalizeIP(s string) string {
	ip := strings.TrimSpace(s)
	if ip == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(ip)
	if err == nil && host != "" {
		ip = host
	}
	ip = strings.Trim(ip, "[]")
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

func SetKliveClientInfoByStream(ctx context.Context, streamID string, info *klivemod.ClientInfo, ttl time.Duration) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" || info == nil {
		return nil
	}
	info.IP = normalizeIP(info.IP)

	b, _ := json.Marshal(info)
	return config.Redis.Set(ctx, kliveStreamKey(streamID), string(b), ttl).Err()
}

func GetKliveClientInfoByStream(ctx context.Context, streamID string) (*klivemod.ClientInfo, bool, error) {
	if config.Redis == nil {
		return nil, false, errors.New("redis not initialized")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil, false, nil
	}

	s, err := config.Redis.Get(ctx, kliveStreamKey(streamID)).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return nil, false, nil
		}
		return nil, false, err
	}

	// backward compat: ถ้าเคยเก็บเป็น ip ล้วน
	s2 := strings.TrimSpace(s)
	if s2 != "" && s2[0] != '{' {
		ip := normalizeIP(s2)
		if ip == "" {
			_ = config.Redis.Del(ctx, kliveStreamKey(streamID)).Err()
			return nil, false, nil
		}
		return &klivemod.ClientInfo{IP: ip}, true, nil
	}

	var ci klivemod.ClientInfo
	if err := json.Unmarshal([]byte(s), &ci); err != nil {
		_ = config.Redis.Del(ctx, kliveStreamKey(streamID)).Err()
		return nil, false, nil
	}
	ci.IP = normalizeIP(ci.IP)
	return &ci, true, nil
}

func DeleteKliveClientInfoByStream(ctx context.Context, streamID string) error {
	if config.Redis == nil {
		return errors.New("redis not initialized")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil
	}
	return config.Redis.Del(ctx, kliveStreamKey(streamID)).Err()
}
