// internal/services/klivesvc/watcherStream.go
package klivesvc

import (
	"context"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/cachego/cacheklive"
)

const (
	streamPrefix    = "klive:clientinfo:stream:"
	kickLockPref    = "lock:klive:kick:stream:" // distributed lock
	defaultRTSPPort = 554                       // ปรับได้ หรือส่งเป็น env/param
)

// StartStreamExpiryWatcher listens to redis expired events and kicks ZLM sessions by peer_ip.
// IMPORTANT: ต้องเปิด notify-keyspace-events = Ex ใน redis
func StartStreamExpiryWatcher(ctx context.Context, podID string) {
	log := logger.FromCtx(ctx, "klivesvc", "StreamExpiryWatcher")

	if config.Redis == nil {
		log.Warn().Msg("redis not ready, watcher disabled")
		return
	}

	db := config.Redis.Options().DB
	channel := "__keyevent@" + itoa(db) + "__:expired"
	sub := config.Redis.Subscribe(ctx, channel)

	log.Info().
		Str("channel", channel).
		Str("pod", podID).
		Msg("✅ stream expiry watcher started")

	go func() {
		defer sub.Close()

		for {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(150 * time.Millisecond)
				continue
			}

			key := strings.TrimSpace(msg.Payload)
			if !strings.HasPrefix(key, streamPrefix) {
				continue
			}

			stream := strings.TrimPrefix(key, streamPrefix)
			stream = strings.TrimSpace(stream)
			if stream == "" {
				continue
			}

			// 🔐 distributed lock กันหลาย replicas ทำซ้ำ
			lockKey := kickLockPref + stream
			ok, _ := config.Redis.SetNX(ctx, lockKey, podID, 20*time.Second).Result()
			if !ok {
				continue
			}

			// NOTE:
			// key expired แล้ว -> เราอ่าน clientInfo จาก redis ไม่ได้แล้ว
			// ทางแก้ clean: เก็บ peer_ip ไว้ "แยก key" ที่ TTL เดียวกัน แล้ว watcher อ่านจาก key นั้น
			// เพื่อไม่ต้องพึ่ง session อีก
			peerIP, ok2, _ := cacheklive.GetKlivePeerIPByStream(ctx, stream)
			peerIP = strings.TrimSpace(peerIP)
			if !ok2 || peerIP == "" {
				log.Warn().Str("stream", stream).Msg("stream expired but no peer_ip mapping, skip kick")
				continue
			}

			// 🔐 lock ต่อ peer_ip (กันยิงซ้ำจากหลาย stream / หลาย replica)
			ipLockKey := "lock:klive:kick:peerip:" + peerIP
			ok, _ = config.Redis.SetNX(ctx, ipLockKey, podID, 1*time.Second).Result()
			if !ok {
				continue
			}

			if !ok {
				// เพิ่ง kick ip นี้ไปแล้ว (จาก stream อื่น) ไม่ต้องทำซ้ำ
				continue
			}

			log.Info().
				Str("stream", stream).
				Str("peer_ip", peerIP).
				Msg("⏱ stream expired → kick_sessions by peer_ip")

			hit, err := KillZLMByPeerIP(peerIP)
			if err != nil {
				log.Warn().Err(err).Str("peer_ip", peerIP).Msg("kick failed")
			}
			if hit == 0 {
				log.Warn().Str("peer_ip", peerIP).Msg("kick hit=0 (no sessions matched)")
			}

			// cleanup mapping key
			_ = cacheklive.DeleteKlivePeerIPByStream(ctx, stream)
		}
	}()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	// simple int to string without strconv import
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + (i % 10))
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
