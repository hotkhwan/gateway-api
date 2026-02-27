// internal/services/ingestsvc/ingest.go
package ingestsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	negativeCacheTTL = 10 * time.Second // negative cache 10s for org not found
	redisExpireSec   = 2 * time.Second
	redisExpireMin   = 2 * time.Minute
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

// cachedOrgPolicy เก็บ effective policy สำหรับ ingest (includes tenantId)
type cachedOrgPolicy struct {
	Exists           bool   `json:"exists"`
	TenantId         string `json:"tenantId"`
	MaxPayloadBytes  int64  `json:"maxPayloadBytes"`
	RateLimitPerSec  int    `json:"rateLimitPerSec"`
	RateLimitBurst   int    `json:"rateLimitBurst"`
	PerIpPerMin      int    `json:"perIpPerMin"`
	OrgCacheTtlSec   int64  `json:"orgCacheTtlSec"`
}

// localEmergencyLimiter provides per-process emergency limiting when Redis is down
type localEmergencyLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newLocalEmergencyLimiter() *localEmergencyLimiter {
	return &localEmergencyLimiter{
		counts: make(map[string]int),
	}
}

func (l *localEmergencyLimiter) Check(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keyWithTime := fmt.Sprintf("%s:%d", key, now.Unix())
	l.counts[keyWithTime]++
	return l.counts[keyWithTime] <= limit
}

// IngestService — thin hot-path service
// ไม่มี auth, ไม่ยุ่ง Permify
// Updated to use subscription-based limits
type IngestService struct {
	orgRepo  *authzrepo.OrgRepo
	subSvc   *subscriptionsvc.SubscriptionService
	redis    *redis.Client
	localOrg *localEmergencyLimiter
	logger   zerolog.Logger
}

func NewIngestService(
	orgRepo *authzrepo.OrgRepo,
	subSvc *subscriptionsvc.SubscriptionService,
	redis *redis.Client,
	logger zerolog.Logger,
) *IngestService {
	if orgRepo == nil || subSvc == nil || redis == nil {
		panic("IngestService: orgRepo, subSvc, redis and logger are required")
	}
	return &IngestService{
		orgRepo:  orgRepo,
		subSvc:   subSvc,
		redis:    redis,
		localOrg: newLocalEmergencyLimiter(),
		logger:   logger,
	}
}

// resolveOrgPolicy returns effective ingest policy for an org
// This includes subscription limits + org overrides
func (s *IngestService) resolveOrgPolicy(ctx context.Context, orgId string) (*cachedOrgPolicy, error) {
	// Cache key includes version for future schema changes
	key := fmt.Sprintf("orgcache:ingest:v1:%s", orgId)

	// Try cache first
	if val, err := s.redis.Get(ctx, key).Bytes(); err == nil {
		var policy cachedOrgPolicy
		if json.Unmarshal(val, &policy) == nil {
			s.logger.Debug().
				Str("orgId", orgId).
				Str("tenantId", policy.TenantId).
				Str("planId", "cached").
				Bool("cacheHit", true).
				Msg("org policy cache hit")
			return &policy, nil
		}
	}

	s.logger.Debug().
		Str("orgId", orgId).
		Bool("cacheHit", false).
		Msg("org policy cache miss")

	// Cache miss → fetch from Mongo
	org, err := s.orgRepo.FindById(ctx, orgId)
	if err != nil {
		// Negative cache 10s to prevent hammering Mongo
		empty, _ := json.Marshal(&cachedOrgPolicy{Exists: false})
		_ = s.redis.Set(ctx, key, empty, negativeCacheTTL).Err()
		return &cachedOrgPolicy{Exists: false}, nil
	}

	// Get tenant subscription limits
	limits, subErr := s.subSvc.GetTenantLimitsCached(ctx, org.TenantId)
	if subErr != nil {
		// If subscription unavailable, return error (fail-closed)
		s.logger.Error().
			Str("orgId", orgId).
			Str("tenantId", org.TenantId).
			Err(subErr).
			Msg("failed to get tenant limits")
		return nil, ErrSubscriptionUnavailable
	}

	// Merge org config with subscription limits
	// Safety: org config cannot exceed subscription limits
	rateLimitPerSec := limits.PerOrgPerSec
	rateLimitBurst := limits.PerOrgBurst
	if org.IngestConfig.RateLimitPerSec > 0 && org.IngestConfig.RateLimitPerSec <= limits.PerOrgPerSec {
		rateLimitPerSec = org.IngestConfig.RateLimitPerSec
	}
	if org.IngestConfig.RateLimitBurst > 0 && org.IngestConfig.RateLimitBurst <= limits.PerOrgBurst {
		rateLimitBurst = org.IngestConfig.RateLimitBurst
	}

	policy := &cachedOrgPolicy{
		Exists:           true,
		TenantId:         org.TenantId,
		MaxPayloadBytes:  limits.MaxPayloadBytes,
		RateLimitPerSec:  rateLimitPerSec,
		RateLimitBurst:   rateLimitBurst,
		PerIpPerMin:      limits.PerIpPerMin,
		OrgCacheTtlSec:   limits.OrgCacheTtlSec,
	}

	s.logger.Debug().
		Str("orgId", orgId).
		Str("tenantId", org.TenantId).
		Str("planId", limits.PlanId).
		Int64("maxPayloadBytes", limits.MaxPayloadBytes).
		Int("perOrgPerSec", limits.PerOrgPerSec).
		Int("perIpPerMin", limits.PerIpPerMin).
		Msg("org policy resolved")

	// Cache with TTL from plan
	cacheTTL := time.Duration(limits.OrgCacheTtlSec) * time.Second
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second // Default 30s
	}

	b, _ := json.Marshal(policy)
	_ = s.redis.Set(ctx, key, b, cacheTTL).Err()

	return policy, nil
}

// checkRateLimit checks rate limit per-org and per-IP
// Uses Redis counters with fail-open + local emergency limiter
func (s *IngestService) checkRateLimit(
	ctx context.Context,
	tenantId, orgId, ip string,
	perSec, perIpPerMin int,
) error {
	now := time.Now().Unix()

	// Per-org rate limit
	orgKey := fmt.Sprintf("rl:org:%s:%s:%d", tenantId, orgId, now)
	orgCnt, err := s.redis.Incr(ctx, orgKey).Result()
	if err == nil {
		if orgCnt == 1 {
			_ = s.redis.Expire(ctx, orgKey, redisExpireSec).Err()
		}
		if orgCnt > int64(perSec) {
			s.logger.Warn().
				Str("orgId", orgId).
				Str("tenantId", tenantId).
				Int64("count", orgCnt).
				Int("limit", perSec).
				Str("reason", "per-org").
				Msg("rate limited: per-org limit exceeded")
			return ErrRateLimited
		}
	} else {
		// Redis down: use local emergency limiter
		if !s.localOrg.Check(orgKey, perSec) {
			s.logger.Warn().
				Str("orgId", orgId).
				Str("tenantId", tenantId).
				Str("reason", "redis-down-emergency").
				Msg("rate limited: local emergency limiter (Redis down)")
			return ErrRateLimited
		}
	}

	// Per-IP rate limit (scoped by tenant to prevent cross-tenant impact)
	ipKey := fmt.Sprintf("rl:ip:%s:%s:%d", tenantId, ip, now/60)
	ipCnt, err := s.redis.Incr(ctx, ipKey).Result()
	if err == nil {
		if ipCnt == 1 {
			_ = s.redis.Expire(ctx, ipKey, redisExpireMin).Err()
		}
		if ipCnt > int64(perIpPerMin) {
			s.logger.Warn().
				Str("orgId", orgId).
				Str("tenantId", tenantId).
				Str("ip", ip).
				Int64("count", ipCnt).
				Int("limit", perIpPerMin).
				Str("reason", "per-ip").
				Msg("rate limited: per-IP limit exceeded")
			return ErrRateLimited
		}
	} else {
		// Redis down: use local emergency limiter
		if !s.localOrg.Check(ipKey, perIpPerMin) {
			s.logger.Warn().
				Str("orgId", orgId).
				Str("tenantId", tenantId).
				Str("ip", ip).
				Str("reason", "redis-down-emergency").
				Msg("rate limited: local emergency limiter (Redis down)")
			return ErrRateLimited
		}
	}

	return nil
}

// Ingest is the main entry point: validate → rate limit → produce Kafka
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

	// 1) Resolve org policy (includes subscription limits)
	policy, err := s.resolveOrgPolicy(ctx, orgId)
	if err != nil {
		return nil, err
	}
	if !policy.Exists {
		return nil, ErrOrgNotFound
	}

	// 2) Payload size check using subscription limit
	if int64(len(body)) > policy.MaxPayloadBytes {
		s.logger.Warn().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Int64("payloadSize", int64(len(body))).
			Int64("maxPayloadBytes", policy.MaxPayloadBytes).
			Msg("payload too large")
		return nil, ErrPayloadTooLarge
	}

	// 3) Rate limit check using subscription limits
	if err := s.checkRateLimit(
		ctx,
		policy.TenantId,
		orgId,
		sourceIp,
		policy.RateLimitPerSec,
		policy.PerIpPerMin,
	); err != nil {
		return nil, err
	}

	// 4) Build raw event
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

	// 5) Produce Kafka (key = orgId → same partition per org)
	topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, nil); err != nil {
		s.logger.Error().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("kafka produce failed")
		return nil, fmt.Errorf("kafka produce failed: %w", err)
	}

	s.logger.Debug().
		Str("orgId", orgId).
		Str("tenantId", policy.TenantId).
		Str("eventId", eventId).
		Str("planId", "from-policy").
		Int64("payloadSize", int64(len(body))).
		Msg("ingest accepted")

	return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
}
