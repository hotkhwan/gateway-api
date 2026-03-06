// internal/services/eventsvc/ingest.go
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
	"github.com/hotkhwan/gateway-api/internal/repo/cacheevt"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestmgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
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
	EventId     string    `json:"eventId"`
	ReceivedAt  time.Time `json:"receivedAt"`
	DeviceKey   string    `json:"deviceKey,omitempty"`   // Canonical device key (e.g., "camera:cam-001")
	Locked      bool      `json:"locked,omitempty"`      // True if device has pending event
	LockMessage string    `json:"lockMessage,omitempty"` // Lock reason if locked
}

// cachedOrgPolicy เก็บ effective policy สำหรับ ingest (includes tenantId)
type cachedOrgPolicy struct {
	Exists          bool   `json:"exists"`
	TenantId        string `json:"tenantId"`
	MaxPayloadBytes int64  `json:"maxPayloadBytes"`
	RateLimitPerSec int    `json:"rateLimitPerSec"`
	RateLimitBurst  int    `json:"rateLimitBurst"`
	PerIpPerMin     int    `json:"perIpPerMin"`
	OrgCacheTtlSec  int64  `json:"orgCacheTtlSec"`
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
// Updated to use subscription-based limits and event management
type IngestService struct {
	orgRepo          *authzrepo.OrgRepo
	eventMgmtRepo    *ingestmgmtrepo.EventManagementRepo
	eventDetailsRepo *ingestdetailsrepo.EventDetailsRepo
	subSvc           *subscriptionsvc.SubscriptionService
	redis            *redis.Client
	localOrg         *localEmergencyLimiter
	logger           zerolog.Logger
}

func NewIngestService(
	orgRepo *authzrepo.OrgRepo,
	eventMgmtRepo *ingestmgmtrepo.EventManagementRepo,
	eventDetailsRepo *ingestdetailsrepo.EventDetailsRepo,
	subSvc *subscriptionsvc.SubscriptionService,
	redis *redis.Client,
	logger zerolog.Logger,
) *IngestService {
	if orgRepo == nil || eventMgmtRepo == nil || eventDetailsRepo == nil || subSvc == nil || redis == nil {
		panic("IngestService: orgRepo, eventMgmtRepo, eventDetailsRepo, subSvc, redis and logger are required")
	}
	return &IngestService{
		orgRepo:          orgRepo,
		eventMgmtRepo:    eventMgmtRepo,
		eventDetailsRepo: eventDetailsRepo,
		subSvc:           subSvc,
		redis:            redis,
		localOrg:         newLocalEmergencyLimiter(),
		logger:           logger,
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
		Exists:          true,
		TenantId:        org.TenantId,
		MaxPayloadBytes: limits.MaxPayloadBytes,
		RateLimitPerSec: rateLimitPerSec,
		RateLimitBurst:  rateLimitBurst,
		PerIpPerMin:     limits.PerIpPerMin,
		OrgCacheTtlSec:  limits.OrgCacheTtlSec,
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

// Ingest is main entry point: validate → rate limit → store pending event
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

	// 4) Generate event ID and timestamps
	eventId := uuid.NewString()
	receivedAt := time.Now().UTC()

	// 5) Auto-detect event type
	suggestedType := s.detectEventType(body)

	// 5) Normalize device identity
	deviceRef, rawAliases, _ := s.normalizeDeviceIdentity(body)
	deviceKey := s.computeDeviceKey(deviceRef)
	_ = rawAliases // Store rawAliases but don't use it (for audit purposes)

	// 6) Check if device:eventType is already approved (skip approval flow)
	isApproved := cacheevt.IsDeviceEventTypeApproved(ctx, policy.TenantId, deviceKey, suggestedType)
	if isApproved && deviceKey != "" {
		s.logger.Info().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Str("deviceKey", deviceKey).
			Str("eventType", suggestedType).
			Msg("device:eventType already approved, auto-processing")

		// Auto-process: Normalize, store in event_details, send to Kafka
		if err := s.processAutoApprovedEvent(
			ctx,
			policy.TenantId,
			orgId,
			deviceKey,
			suggestedType,
			eventId,
			receivedAt,
			sourceIp,
			body,
			rawAliases,
			deviceRef,
		); err != nil {
			s.logger.Error().
				Str("orgId", orgId).
				Str("tenantId", policy.TenantId).
				Err(err).
				Msg("failed to auto-process approved event")
			return nil, fmt.Errorf("failed to auto-process event: %w", err)
		}

		return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
	}

	// 7) Check for device pending lock (same device has pending event)
	isLocked, pendingEventId, err := s.checkDevicePendingLock(ctx, policy.TenantId, orgId, deviceKey)
	if err != nil {
		s.logger.Error().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Err(err).
			Msg("failed to check device pending lock")
		return nil, fmt.Errorf("failed to check device lock: %w", err)
	}

	// 8) Build pending event (or return locked response)
	if isLocked {
		s.logger.Warn().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Str("deviceKey", deviceKey).
			Str("pendingEventId", pendingEventId).
			Msg("device has pending event, rejecting new event")

		return &IngestResult{
			EventId:     eventId,
			ReceivedAt:  receivedAt,
			DeviceKey:   deviceKey,
			Locked:      true,
			LockMessage: fmt.Sprintf("Device has pending event: %s", pendingEventId),
		}, nil
	}

	pendingEvent := &ingestmod.EventManagement{
		EventId:       eventId,
		TenantId:      policy.TenantId,
		OrgId:         orgId,
		Name:          fmt.Sprintf("Event %s", eventId[:8]),
		Lat:           0,
		Lng:           0,
		EventType:     suggestedType,
		Status:        false,
		StatusName:    "pending",
		RawBody:       json.RawMessage(body),
		ContentType:   contentType,
		SourceIp:      sourceIp,
		SuggestedType: suggestedType,
		DeviceRef:     deviceRef,
		DeviceKey:     deviceKey,
		RawAliases:    rawAliases,
		CreatedAt:     receivedAt,
		UpdatedAt:     receivedAt,
	}

	if err := s.eventMgmtRepo.Insert(ctx, pendingEvent); err != nil {
		s.logger.Error().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("failed to store pending event")
		return nil, fmt.Errorf("failed to store event: %w", err)
	}

	// 9) Cache event status in Redis
	if err := cacheevt.SetEventStatusPending(ctx, policy.TenantId, eventId); err != nil {
		// Log warning but don't fail - this is non-critical
		s.logger.Warn().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("failed to cache event status in Redis (non-critical)")
	}

	s.logger.Debug().
		Str("orgId", orgId).
		Str("tenantId", policy.TenantId).
		Str("eventId", eventId).
		Str("deviceKey", deviceKey).
		Str("suggestedType", suggestedType).
		Int64("payloadSize", int64(len(body))).
		Msg("ingest stored as pending event")

	return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
}

// processAutoApprovedEvent handles events that have pre-approved device:eventType
// Normalizes, stores in event_details, and sends to Kafka
func (s *IngestService) processAutoApprovedEvent(
	ctx context.Context,
	tenantId, orgId, deviceKey, eventType, eventId string,
	receivedAt time.Time,
	sourceIp string,
	body []byte,
	_ json.RawMessage,
	_ *ingestmod.DeviceIdentity,
) error {
	now := time.Now().UTC()

	// 1) Normalize event data
	normalizedData, err := s.normalizeEvent(eventType, json.RawMessage(body))
	if err != nil {
		return fmt.Errorf("normalization failed: %w", err)
	}

	// 2) Create approved event detail
	eventDetail := &ingestmod.EventDetail{
		EventId:        eventId,
		TenantId:       tenantId,
		OrgId:          orgId,
		Name:           fmt.Sprintf("Event %s (Auto-approved)", eventId[:8]),
		Lat:            0,
		Lng:            0,
		EventType:      eventType,
		NormalizedData: normalizedData,
		SourceIp:       sourceIp,
		IngestedAt:     receivedAt,
		ApprovedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// 3) Store in event_details
	if err := s.eventDetailsRepo.Insert(ctx, eventDetail); err != nil {
		return fmt.Errorf("failed to store approved event: %w", err)
	}

	// 4) Send to Kafka (normalized topic)
	topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
	payload, _ := json.Marshal(eventDetail)
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, nil); err != nil {
		// Log error but don't block processing
		s.logger.Error().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Str("deviceKey", deviceKey).
			Str("eventType", eventType).
			Err(err).
			Msg("kafka send failed (non-blocking)")
	}

	// 5) Cache event status as approved
	if err := cacheevt.SetEventStatusApproved(ctx, tenantId, eventId); err != nil {
		s.logger.Warn().
			Str("tenantId", tenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("failed to update Redis cache (non-critical)")
	}

	s.logger.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("deviceKey", deviceKey).
		Str("eventType", eventType).
		Msg("auto-approved event processed successfully")

	return nil
}

// normalizeEvent normalizes raw event data
// For now, returns raw data with metadata
// Future: Apply type-specific normalization templates
func (s *IngestService) normalizeEvent(eventType string, rawBody json.RawMessage) (json.RawMessage, error) {
	result := map[string]any{
		"eventType":  eventType,
		"normalized": true,
		"rawData":    rawBody,
	}

	return json.Marshal(result)
}

// detectEventType auto-detects event type from raw body
// Returns suggested event type based on payload patterns
func (s *IngestService) detectEventType(body []byte) string {
	// Parse raw body to extract hints
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "unknown"
	}

	// Pattern matching logic
	if _, ok := raw["plateNumber"]; ok {
		return "LPR_Brand"
	}
	if _, ok := raw["faceId"]; ok {
		return "FACE_Brand"
	}
	if _, ok := raw["deviceId"]; ok {
		return "camera_Brand"
	}
	if _, ok := raw["cameraId"]; ok {
		return "camera_Brand"
	}
	if _, ok := raw["sensorId"]; ok {
		return "IOT_Brand"
	}

	return "unknown"
}

// normalizeDeviceIdentity normalizes device identity from raw event body
// Returns: deviceRef (nil if missing), rawAliasesBytes (json), aliases map
func (s *IngestService) normalizeDeviceIdentity(body []byte) (*ingestmod.DeviceIdentity, json.RawMessage, map[string]string) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, nil
	}

	aliases := map[string]string{}

	getString := func(v any) (string, bool) {
		switch t := v.(type) {
		case string:
			if t == "" {
				return "", false
			}
			return t, true
		default:
			return "", false
		}
	}

	// helper: find first existing key in raw (for alias support)
	findFirst := func(keys ...string) (key string, val string, ok bool) {
		for _, k := range keys {
			v, exists := raw[k]
			if !exists {
				continue
			}
			s, ok := getString(v)
			if !ok {
				continue
			}
			return k, s, true
		}
		return "", "", false
	}

	// Priority rules (stop at first match):
	// 1) cameraId -> camera
	// 2) sensorId -> sensor
	// 3) faceId   -> face
	// 4) deviceId -> device
	if k, id, ok := findFirst("cameraId", "camId", "camera_id"); ok {
		aliases[k] = id
		rawAliasesBytes, _ := json.Marshal(aliases)
		return &ingestmod.DeviceIdentity{Type: "camera", ID: id}, rawAliasesBytes, aliases
	}

	if k, id, ok := findFirst("sensorId", "sensor_id"); ok {
		aliases[k] = id
		rawAliasesBytes, _ := json.Marshal(aliases)
		return &ingestmod.DeviceIdentity{Type: "sensor", ID: id}, rawAliasesBytes, aliases
	}

	if k, id, ok := findFirst("faceId", "face_id"); ok {
		aliases[k] = id
		rawAliasesBytes, _ := json.Marshal(aliases)
		return &ingestmod.DeviceIdentity{Type: "face", ID: id}, rawAliasesBytes, aliases
	}

	if k, id, ok := findFirst("deviceId", "device_id"); ok {
		aliases[k] = id
		rawAliasesBytes, _ := json.Marshal(aliases)
		return &ingestmod.DeviceIdentity{Type: "device", ID: id}, rawAliasesBytes, aliases
	}

	// no identity
	rawAliasesBytes, _ := json.Marshal(aliases)
	return nil, rawAliasesBytes, aliases
}

// computeDeviceKey computes canonical device key for locking
// Format: "type:id" (e.g., "camera:cam-001", "device:dev-001")
func (s *IngestService) computeDeviceKey(deviceRef *ingestmod.DeviceIdentity) string {
	if deviceRef == nil || deviceRef.Type == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", deviceRef.Type, deviceRef.ID)
}

// checkDevicePendingLock checks if device has pending events
// Returns (isLocked, pendingEventId, error)
func (s *IngestService) checkDevicePendingLock(ctx context.Context, tenantId, orgId, deviceKey string) (bool, string, error) {
	if deviceKey == "" {
		return false, "", nil
	}

	// Check for existing pending event with same deviceKey in pending status
	// Use partial unique index for efficient lookup
	pending, err := s.eventMgmtRepo.FindByDeviceKey(ctx, tenantId, orgId, deviceKey)
	if err != nil {
		// ErrNotFound means no pending event exists (no lock), not an error
		if err == ingestmgmtrepo.ErrNotFound {
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to check device lock: %w", err)
	}

	if pending != nil {
		return true, pending.EventId, nil
	}

	return false, "", nil
}
