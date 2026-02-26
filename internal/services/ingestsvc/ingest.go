// internal/services/ingestsvc/ingest.go
package ingestsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/redis/go-redis/v9"
)

const (
	orgCacheTTL     = 30 * time.Second
	MaxPayloadBytes = 256 * 1024 // 256 KB
	defaultPerSec   = 10
	defaultIPPerMin = 300 // per-IP guard: 300 req/min
)

// RawEvent คือ message ที่ส่งเข้า Kafka raw.events
type RawEvent struct {
	OrgId       string          `json:"orgId"`
	EventId     string          `json:"eventId"`
	ReceivedAt  time.Time       `json:"receivedAt"`
	RawBody     json.RawMessage `json:"rawBody"`
	SourceIp    string          `json:"sourceIp"`
	ContentType string          `json:"contentType,omitempty"`
}

// IngestResult คือ response กลับ client
type IngestResult struct {
	EventId    string    `json:"eventId"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// cachedOrg เก็บ org config ใน Redis (TTL 30s)
type cachedOrg struct {
	Exists          bool   `json:"exists"`
	RateLimitPerSec int    `json:"rateLimitPerSec"`
	RateLimitBurst  int    `json:"rateLimitBurst"`
}

// IngestService — thin hot-path service
// ไม่มี auth, ไม่ยุ่ง Permify
type IngestService struct {
	orgRepo *authzrepo.OrgRepo
	redis   *redis.Client
}

func NewIngestService(orgRepo *authzrepo.OrgRepo, redis *redis.Client) *IngestService {
	if orgRepo == nil || redis == nil {
		panic("IngestService: orgRepo and redis are required")
	}
	return &IngestService{orgRepo: orgRepo, redis: redis}
}

// lookupOrg คืน org config จาก Redis cache หรือ Mongo fallback
func (s *IngestService) lookupOrg(ctx context.Context, orgId string) (*cachedOrg, error) {
	key := fmt.Sprintf("orgcache:ingest:%s", orgId)

	// try cache first
	if val, err := s.redis.Get(ctx, key).Bytes(); err == nil {
		var cached cachedOrg
		if json.Unmarshal(val, &cached) == nil {
			return &cached, nil
		}
	}

	// cache miss → Mongo
	org, err := s.orgRepo.FindById(ctx, orgId)
	if err != nil {
		// negative cache 10s เพื่อกัน org ที่ไม่มีถล่ม Mongo
		empty, _ := json.Marshal(&cachedOrg{Exists: false})
		_ = s.redis.Set(ctx, key, empty, 10*time.Second).Err()
		return &cachedOrg{Exists: false}, nil
	}

	cfg := &cachedOrg{
		Exists:          true,
		RateLimitPerSec: org.IngestConfig.RateLimitPerSec,
		RateLimitBurst:  org.IngestConfig.RateLimitBurst,
	}
	b, _ := json.Marshal(cfg)
	_ = s.redis.Set(ctx, key, b, orgCacheTTL).Err()
	return cfg, nil
}

// checkRateLimit ตรวจ rate limit per-org (fixed window/sec) และ per-IP (fixed window/min)
// fail-open: ถ้า Redis ล่มให้ผ่านไปก่อน
func (s *IngestService) checkRateLimit(ctx context.Context, orgId, ip string, perSec int) error {
	now := time.Now().Unix()

	// --- per-org ---
	if perSec <= 0 {
		perSec = defaultPerSec
	}
	orgKey := fmt.Sprintf("rl:org:%s:%d", orgId, now)
	cnt, err := s.redis.Incr(ctx, orgKey).Result()
	if err == nil {
		if cnt == 1 {
			_ = s.redis.Expire(ctx, orgKey, 2*time.Second).Err()
		}
		if cnt > int64(perSec) {
			return ErrRateLimited
		}
	}

	// --- per-IP ---
	minKey := fmt.Sprintf("rl:ip:%s:%d", ip, now/60)
	ipCnt, err := s.redis.Incr(ctx, minKey).Result()
	if err == nil {
		if ipCnt == 1 {
			_ = s.redis.Expire(ctx, minKey, 2*time.Minute).Err()
		}
		if ipCnt > defaultIPPerMin {
			return ErrRateLimited
		}
	}

	return nil
}

// Ingest คือ main entry point: validate → rate limit → produce Kafka
func (s *IngestService) Ingest(
	ctx context.Context,
	orgId string,
	sourceIp string,
	contentType string,
	body []byte,
) (*IngestResult, error) {

	if len(body) == 0 {
		return nil, ErrEmptyBody
	}
	if len(body) > MaxPayloadBytes {
		return nil, ErrPayloadTooLarge
	}

	// 1) lookup org (cache + Mongo)
	org, err := s.lookupOrg(ctx, orgId)
	if err != nil || !org.Exists {
		return nil, ErrOrgNotFound
	}

	// 2) rate limit
	if err := s.checkRateLimit(ctx, orgId, sourceIp, org.RateLimitPerSec); err != nil {
		return nil, err
	}

	// 3) build raw event
	eventId := uuid.NewString()
	receivedAt := time.Now().UTC()

	raw := RawEvent{
		OrgId:       orgId,
		EventId:     eventId,
		ReceivedAt:  receivedAt,
		RawBody:     json.RawMessage(body),
		SourceIp:    sourceIp,
		ContentType: contentType,
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	// 4) produce Kafka (key = orgId → same partition per org)
	topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, nil); err != nil {
		return nil, fmt.Errorf("kafka produce failed: %w", err)
	}

	return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
}
