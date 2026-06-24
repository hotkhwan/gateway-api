// internal/services/ingestsvc/ingest.go
package ingestsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/cacheevt"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestmgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/workspacerepo"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
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
	WorkspaceId  string          `json:"workspaceId"`
	EventId      string          `json:"eventId"`
	SourceFamily string          `json:"sourceFamily"`
	ReceivedAt   time.Time       `json:"receivedAt"`
	RawBody      json.RawMessage `json:"rawBody"`
	SourceIp     string          `json:"sourceIp"`
	ContentType  string          `json:"contentType,omitempty"`
}

// IngestResult คือ response กลับ client
type IngestResult struct {
	EventId     string    `json:"eventId"`
	ReceivedAt  time.Time `json:"receivedAt"`
	DeviceKey   string    `json:"deviceKey,omitempty"`   // Canonical device key (e.g., "camera:cam-001")
	Locked      bool      `json:"locked,omitempty"`      // True if device has pending event
	LockMessage string    `json:"lockMessage,omitempty"` // Lock reason if locked
	Pending     bool      `json:"pending,omitempty"`     // True if event is queued in event_management for review
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
// V3: env-guarded sourceFamily, mode matrix, fingerprint-based review
type IngestService struct {
	workspaceRepo      *workspacerepo.WorkspaceRepo
	eventMgmtRepo      *ingestmgmtrepo.EventManagementRepo
	subSvc             *subscriptionsvc.SubscriptionService
	redis              *redis.Client
	localOrg           *localEmergencyLimiter
	tmplMatcher        *TemplateMatcher
	sourceProfileSvc   SourceProfileResolver
	reviewSvc          ReviewCreator
	rejectedPatternSvc RejectedPatternChecker
	suggestionSvc      SuggestionProvider
	deviceMgmtSvc      DeviceResolver
	enabledFamilies    map[string]bool // parsed from INGEST_ENABLED_SOURCE_FAMILIES
	logger             zerolog.Logger
}

// SourceProfileResolver loads source profiles (injected to avoid circular imports).
type SourceProfileResolver interface {
	Get(ctx context.Context, sourceFamily string) (*ingestmod.SourceProfile, error)
}

// ReviewCreator creates/updates unknown payload reviews (injected to avoid circular imports).
type ReviewCreator interface {
	UpsertIfNotExists(ctx context.Context, orgId, sourceFamily, fingerprint string, samplePayload map[string]any, candidateSuggestionIds []string) string
}

// RejectedPatternChecker checks if a fingerprint has been rejected (injected to avoid circular imports).
type RejectedPatternChecker interface {
	IsRejected(ctx context.Context, orgId, sourceFamily, fingerprint string) bool
}

// SuggestionProvider returns mapping suggestions for a sourceFamily (injected to avoid circular imports).
type SuggestionProvider interface {
	GetByFamily(sourceFamily string) []*ingestmod.MappingSuggestion
	GetByID(id string) *ingestmod.MappingSuggestion
}

// DeviceResolver resolves device enrichment data (injected to avoid circular imports).
type DeviceResolver interface {
	Resolve(ctx context.Context, tenantId, orgId, sourceFamily, entityType, entityId string) *ingestmod.DeviceManagement
	AutoUpsertFromEvent(ctx context.Context, tenantId, orgId, sourceFamily string, ref *ingestmod.DeviceIdentity, hints ingestmod.AutoUpsertHints)
}

func NewIngestService(
	workspaceRepo *workspacerepo.WorkspaceRepo,
	eventMgmtRepo *ingestmgmtrepo.EventManagementRepo,
	subSvc *subscriptionsvc.SubscriptionService,
	redis *redis.Client,
	tmplMatcher *TemplateMatcher,
	sourceProfileSvc SourceProfileResolver,
	reviewSvc ReviewCreator,
	rejectedPatternSvc RejectedPatternChecker,
	suggestionSvc SuggestionProvider,
	deviceMgmtSvc DeviceResolver,
	logger zerolog.Logger,
) *IngestService {
	if workspaceRepo == nil || subSvc == nil || redis == nil {
		panic("IngestService: workspaceRepo, subSvc, redis are required")
	}
	if tmplMatcher == nil {
		panic("IngestService: tmplMatcher is required")
	}
	return &IngestService{
		workspaceRepo:      workspaceRepo,
		eventMgmtRepo:      eventMgmtRepo,
		subSvc:             subSvc,
		redis:              redis,
		localOrg:           newLocalEmergencyLimiter(),
		tmplMatcher:        tmplMatcher,
		sourceProfileSvc:   sourceProfileSvc,
		reviewSvc:          reviewSvc,
		rejectedPatternSvc: rejectedPatternSvc,
		suggestionSvc:      suggestionSvc,
		deviceMgmtSvc:      deviceMgmtSvc,
		enabledFamilies:    parseEnabledFamilies(os.Getenv("INGEST_ENABLED_SOURCE_FAMILIES")),
		logger:             logger,
	}
}

// parseEnabledFamilies parses comma-separated INGEST_ENABLED_SOURCE_FAMILIES env value.
func parseEnabledFamilies(envVal string) map[string]bool {
	result := make(map[string]bool)
	for _, f := range strings.Split(envVal, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			result[f] = true
		}
	}
	return result
}

// isSourceFamilyEnabled checks env allowlist.
func (s *IngestService) isSourceFamilyEnabled(sourceFamily string) bool {
	// If no families configured, reject all (fail-closed)
	if len(s.enabledFamilies) == 0 {
		return false
	}
	return s.enabledFamilies[sourceFamily]
}

// getSourceFamilyMode returns mode from sourceProfile ("active"|"comingSoon"|"mock").
// Defaults to "active" if profile not found.
func (s *IngestService) getSourceFamilyMode(ctx context.Context, sourceFamily string) string {
	if s.sourceProfileSvc == nil {
		return "active"
	}
	profile, err := s.sourceProfileSvc.Get(ctx, sourceFamily)
	if err != nil || profile == nil {
		return "active"
	}
	if profile.Mode == "" {
		return "active"
	}
	return profile.Mode
}

// resolveOrgPolicy returns effective ingest policy for a workspace
// This includes subscription limits from tenantId
func (s *IngestService) resolveOrgPolicy(ctx context.Context, workspaceId string) (*cachedOrgPolicy, error) {
	// Cache key includes version for future schema changes
	key := fmt.Sprintf("orgcache:ingest:v1:%s", workspaceId)

	// Try cache first
	if val, err := s.redis.Get(ctx, key).Bytes(); err == nil {
		var policy cachedOrgPolicy
		if json.Unmarshal(val, &policy) == nil {
			s.logger.Debug().
				Str("orgId", workspaceId).
				Str("tenantId", policy.TenantId).
				Str("planId", "cached").
				Bool("cacheHit", true).
				Msg("org policy cache hit")
			return &policy, nil
		}
	}

	s.logger.Debug().
		Str("orgId", workspaceId).
		Bool("cacheHit", false).
		Msg("org policy cache miss")

	// Cache miss → fetch from Mongo
	ws, err := s.workspaceRepo.FindByWorkspaceID(ctx, workspaceId)
	if err != nil {
		// Negative cache 10s to prevent hammering Mongo
		empty, _ := json.Marshal(&cachedOrgPolicy{Exists: false})
		_ = s.redis.Set(ctx, key, empty, negativeCacheTTL).Err()
		return &cachedOrgPolicy{Exists: false}, nil
	}

	// Reject suspended/archived workspaces
	if ws.Status != workspacemod.WorkspaceActive {
		empty, _ := json.Marshal(&cachedOrgPolicy{Exists: false})
		_ = s.redis.Set(ctx, key, empty, negativeCacheTTL).Err()
		return &cachedOrgPolicy{Exists: false}, nil
	}

	// Get tenant subscription limits
	limits, subErr := s.subSvc.GetTenantLimitsCached(ctx, ws.TenantID)
	if subErr != nil {
		// If subscription unavailable, return error (fail-closed)
		s.logger.Error().
			Str("orgId", workspaceId).
			Str("tenantId", ws.TenantID).
			Err(subErr).
			Msg("failed to get tenant limits")
		return nil, ErrSubscriptionUnavailable
	}

	policy := &cachedOrgPolicy{
		Exists:          true,
		TenantId:        ws.TenantID,
		MaxPayloadBytes: limits.MaxPayloadBytes,
		RateLimitPerSec: limits.PerOrgPerSec,
		RateLimitBurst:  limits.PerOrgBurst,
		PerIpPerMin:     limits.PerIpPerMin,
		OrgCacheTtlSec:  limits.OrgCacheTtlSec,
	}

	s.logger.Debug().
		Str("orgId", workspaceId).
		Str("tenantId", ws.TenantID).
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

// Ingest is main entry point (V3): env guard → mode matrix → rate limit → template match → review
func (s *IngestService) Ingest(
	ctx context.Context,
	orgId string,
	sourceFamily string,
	sourceIp string,
	contentType string,
	body []byte,
) (*IngestResult, error) {

	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	// 1) Env guard: reject sourceFamily not in allowlist
	if !s.isSourceFamilyEnabled(sourceFamily) {
		s.logger.Info().
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Msg("[Ingest] sourceFamily not in enabled allowlist, locked")
		return nil, ErrSourceFamilyLocked
	}

	// 2) Mode check: comingSoon → log body, return early (no DB write)
	mode := s.getSourceFamilyMode(ctx, sourceFamily)
	if mode == "comingSoon" {
		s.logger.Info().
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Int("bodySize", len(body)).
			Str("rawBody", string(body)).
			Msg("[Ingest] sourceFamily comingSoon: payload logged, no DB write")
		return nil, ErrSourceFamilyComingSoon
	}

	// 2b) Multipart normalization (e.g. Dahua multipart/x-mixed-replace):
	//     extract the text/JSON metadata into a single JSON object and reduce
	//     snapshot images to descriptors, so every downstream step (size check,
	//     device extraction, fingerprint, template match) sees clean JSON
	//     instead of a multi-hundred-KB binary blob. Falls back to the original
	//     body if the multipart can't be parsed.
	if isMultipartContentType(contentType) {
		if normalized, ok := normalizeMultipart(contentType, body); ok {
			s.logger.Debug().
				Str("orgId", orgId).
				Str("sourceFamily", sourceFamily).
				Int("rawBytes", len(body)).
				Int("metadataBytes", len(normalized)).
				Msg("[Ingest] multipart normalized to metadata JSON")
			body = normalized
		}
	}

	// 3) Resolve org policy (includes subscription limits)
	policy, err := s.resolveOrgPolicy(ctx, orgId)
	if err != nil {
		return nil, err
	}
	if !policy.Exists {
		return nil, ErrOrgNotFound
	}

	// 4) Payload size check using subscription limit
	if int64(len(body)) > policy.MaxPayloadBytes {
		s.logger.Warn().
			Str("orgId", orgId).
			Str("tenantId", policy.TenantId).
			Int64("payloadSize", int64(len(body))).
			Int64("maxPayloadBytes", policy.MaxPayloadBytes).
			Msg("payload too large")
		return nil, ErrPayloadTooLarge
	}

	// 5) Rate limit check using subscription limits
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

	// 6) Generate event ID and timestamps
	eventId := uuid.NewString()
	receivedAt := time.Now().UTC()

	// 7) Decode rawBody to map
	rawBody, err := decodeRawBody(body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode raw body: %w", err)
	}

	// 8) Build deterministic fingerprint: sorted key paths + normalized value types
	fingerprint := BuildFingerprint(rawBody)

	// 9) Pre-match canonical enrichment — resolve device identity + management record
	//    BEFORE template matching so user-defined matchAll/matchAny can reference
	//    canonical fields (source.deviceId, source.deviceType, ...) that match what
	//    the UI displays — instead of vendor-specific raw fields (channelId vs deviceId).
	candidates, _, _ := s.extractDeviceCandidates(body)
	enrichment, deviceRef := s.resolveDeviceEnrichment(ctx, policy.TenantId, orgId, sourceFamily, candidates)

	// 10) Build match bag = rawBody + canonical "source.*" — passed to template matcher.
	//     Raw fields stay at top level (backward compat); canonical fields under "source.*".
	matchBag := buildMatchBag(rawBody, deviceRef, enrichment, sourceFamily)

	// 11) Template matching: sourceFamily + fingerprint, evaluating against matchBag
	tmpl, matched, matchErr := s.tmplMatcher.MatchByFamily(ctx, policy.TenantId, orgId, sourceFamily, fingerprint, matchBag)
	if matchErr != nil {
		s.logger.Error().Err(matchErr).
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Msg("[Ingest] template match error (non-fatal, creating review)")
	}

	if matched && tmpl != nil {
		// Determine event type from template (fallback: sourceFamily)
		eventType := tmpl.FinalEventType
		if eventType == "" {
			eventType = sourceFamily
		}

		// Auto-create/merge device_management record (conservative, non-blocking)
		if deviceRef != nil && s.deviceMgmtSvc != nil {
			s.deviceMgmtSvc.AutoUpsertFromEvent(ctx, policy.TenantId, orgId, sourceFamily, deviceRef, buildAutoUpsertHints(sourceFamily, rawBody))
		}

		s.logger.Debug().
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Str("eventId", eventId).
			Str("templateId", tmpl.TemplateId).
			Str("eventType", eventType).
			Msg("[Ingest] V2 template matched, publishing")

		if err := s.processTemplateMappedEvent(
			ctx,
			policy.TenantId, orgId, sourceFamily, eventType,
			eventId, receivedAt,
			rawBody, tmpl, deviceRef, enrichment,
		); err != nil {
			return nil, err
		}

		return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
	}

	// 11b) Suggestion fallback: if no DB template matched, try auto-apply from suggestion.
	//      Persists template to DB so users can configure finalEventType, classificationRules,
	//      deliveryTargets, messageTemplates via the management API later.
	if !matched {
		if autoTmpl, suggestionId := s.tryAutoApplySuggestion(ctx, policy.TenantId, orgId, sourceFamily, rawBody); autoTmpl != nil {
			eventType := autoTmpl.FinalEventType
			if eventType == "" {
				eventType = sourceFamily
			}

			// Auto-create/merge device_management record (conservative, non-blocking)
			if deviceRef != nil && s.deviceMgmtSvc != nil {
				s.deviceMgmtSvc.AutoUpsertFromEvent(ctx, policy.TenantId, orgId, sourceFamily, deviceRef, buildAutoUpsertHints(sourceFamily, rawBody))
			}

			s.logger.Info().
				Str("orgId", orgId).
				Str("sourceFamily", sourceFamily).
				Str("eventId", eventId).
				Str("suggestionId", suggestionId).
				Msg("[Ingest] auto-applied suggestion as template")

			if err := s.processTemplateMappedEvent(
				ctx,
				policy.TenantId, orgId, sourceFamily, eventType,
				eventId, receivedAt,
				rawBody, autoTmpl, deviceRef, enrichment,
			); err != nil {
				return nil, err
			}
			return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
		}
	}

	// 10) Check rejected pattern — drop silently if previously rejected
	if s.rejectedPatternSvc != nil && s.rejectedPatternSvc.IsRejected(ctx, orgId, sourceFamily, fingerprint) {
		s.logger.Info().
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Str("fingerprint", fingerprint).
			Msg("[Ingest] payload matches rejected pattern, dropped")
		return &IngestResult{EventId: eventId, ReceivedAt: receivedAt}, nil
	}

	// 11) No template match and not rejected → evaluate suggestions + upsert unknownPayloadReview
	candidateIds := s.findCandidateSuggestions(sourceFamily, rawBody)
	if s.reviewSvc != nil {
		reviewId := s.reviewSvc.UpsertIfNotExists(ctx, orgId, sourceFamily, fingerprint, rawBody, candidateIds)
		if reviewId != "" {
			s.logger.Debug().
				Str("orgId", orgId).
				Str("sourceFamily", sourceFamily).
				Str("fingerprint", fingerprint).
				Str("reviewId", reviewId).
				Strs("candidateSuggestionIds", candidateIds).
				Msg("[Ingest] unknown payload review upserted")
		}
	}

	s.logger.Debug().
		Str("orgId", orgId).
		Str("sourceFamily", sourceFamily).
		Str("fingerprint", fingerprint).
		Msg("[Ingest] no template match, event queued for review")

	return &IngestResult{EventId: eventId, ReceivedAt: receivedAt, Pending: true}, nil
}

// processTemplateMappedEvent applies a matched mapping template to rawBody
// and publishes a CanonicalEvent to raw.events.
// The Normalizer consumer handles writing to event_details + S3.
func (s *IngestService) processTemplateMappedEvent(
	ctx context.Context,
	tenantId, orgId, sourceFamily, eventType, eventId string,
	receivedAt time.Time,
	rawBody map[string]any,
	tmpl *ingestmod.MappingTemplate,
	deviceRef *ingestmod.DeviceIdentity,
	enrichment *ingestmod.DeviceManagement,
) error {
	// 1) Apply field mappings (to extract lat/lng and occurredAt only)
	mapped, missingRequired := s.tmplMatcher.ApplyMappings(rawBody, tmpl.Mappings)
	if len(missingRequired) > 0 {
		s.logger.Warn().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Str("templateId", tmpl.TemplateId).
			Strs("missingRequired", missingRequired).
			Msg("[TemplateMapped] required fields missing in rawBody")
	}

	// 2) Resolve lat/lng from mapped fields
	lat, _ := getNestedValue(mapped, "location.lat")
	lng, _ := getNestedValue(mapped, "location.lng")
	latF, _ := lat.(float64)
	lngF, _ := lng.(float64)

	// 3) Resolve occurredAt from mapped fields (fallback: receivedAt)
	occurredAt := receivedAt
	if ts, ok := getNestedValue(mapped, "occurredAt"); ok {
		if t, ok2 := ts.(time.Time); ok2 {
			occurredAt = t
		}
	}

	// 4) Build location — deviceManagement overwrite applied AFTER field mapping (V3 Step 7)
	location := ingestmod.LocationInfo{Lat: latF, Lng: lngF}
	deviceName := ""
	deviceDescription := ""
	if enrichment != nil {
		if enrichment.Lat != 0 || enrichment.Lng != 0 {
			location.Lat = enrichment.Lat
			location.Lng = enrichment.Lng
		}
		if enrichment.Site != "" {
			location.Site = enrichment.Site
		}
		if enrichment.Zone != "" {
			location.Zone = enrichment.Zone
		}
		if enrichment.Name != "" {
			deviceName = enrichment.Name
		}
		if enrichment.Description != "" {
			deviceDescription = enrichment.Description
		}
		if enrichment.DeviceId != "" && deviceRef != nil {
			deviceRef.ID = enrichment.DeviceId
		}
	}

	// 5) Build CanonicalEvent for raw.events
	deviceId := ""
	deviceType := ""
	if deviceRef != nil {
		deviceId = deviceRef.ID
		deviceType = deviceRef.Type
	}
	canonical := &ingestmod.CanonicalEvent{
		EventId:      eventId,
		TenantId:     tenantId,
		SourceFamily: sourceFamily,
		EventType:    eventType,
		OccurredAt:   occurredAt,
		Source: ingestmod.SourceInfo{
			DeviceId:          deviceId,
			DeviceType:        deviceType,
			DeviceName:        deviceName,
			DeviceDescription: deviceDescription,
			WorkspaceId:       orgId,
		},
		Location:   location,
		Payload:    rawBody, // raw payload — normalizer applies template again
		TemplateId: tmpl.TemplateId,
		CreatedAt:  time.Now().UTC(),
	}

	// 5) Publish to raw.events — normalizer writes to event_details
	topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
	payload, _ := json.Marshal(canonical)
	headers := map[string]string{
		"eventId":      eventId,
		"eventType":    eventType,
		"sourceFamily": sourceFamily,
		"workspaceId":  orgId,
		"tenantId":     tenantId,
		"templateId":   tmpl.TemplateId,
	}
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, headers); err != nil {
		s.logger.Error().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Str("templateId", tmpl.TemplateId).
			Err(err).
			Msg("[TemplateMapped] kafka publish to raw.events failed")
		return fmt.Errorf("kafka publish failed: %w", err)
	}

	// 6) Cache processing state (technical only, not business approval)
	_ = cacheevt.SetEventStatusApproved(ctx, tenantId, eventId)

	s.logger.Debug().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("sourceFamily", sourceFamily).
		Str("eventId", eventId).
		Str("templateId", tmpl.TemplateId).
		Str("templateName", tmpl.Name).
		Str("eventType", eventType).
		Msg("[TemplateMapped] published to raw.events")

	return nil
}

// extractDeviceCandidates extracts all device identity candidates from raw event body.
// Returns candidates in priority order, aliases JSON bytes, and aliases map.
func (s *IngestService) extractDeviceCandidates(body []byte) ([]ingestmod.DeviceIdentity, json.RawMessage, map[string]string) {
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
		case float64: // JSON numbers always decode as float64
			if t == 0 {
				return "", false
			}
			return fmt.Sprintf("%.0f", t), true
		case int:
			if t == 0 {
				return "", false
			}
			return fmt.Sprintf("%d", t), true
		default:
			return "", false
		}
	}

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

	// Priority order — collect ALL matching candidates
	// 1) cameraId  -> camera
	// 2) sensorId  -> sensor
	// 3) faceId    -> face
	// 4) channelId -> channel
	// 5) deviceId  -> device
	type rule struct {
		typ  string
		keys []string
	}
	rules := []rule{
		{"camera", []string{"cameraId", "camId", "camera_id"}},
		{"sensor", []string{"sensorId", "sensor_id"}},
		{"face", []string{"faceId", "face_id"}},
		{"channel", []string{"channelId", "channel_id"}},
		{"device", []string{"deviceId", "device_id", "device"}},
	}

	var candidates []ingestmod.DeviceIdentity
	for _, r := range rules {
		if k, id, ok := findFirst(r.keys...); ok {
			aliases[k] = id
			candidates = append(candidates, ingestmod.DeviceIdentity{Type: r.typ, ID: id})
		}
	}

	rawAliasesBytes, _ := json.Marshal(aliases)
	return candidates, rawAliasesBytes, aliases
}

// resolveDeviceEnrichment tries each candidate in priority order against deviceManagement.
// Returns the first matched enrichment and the winning deviceRef.
// If no enrichment is found, returns nil enrichment and the first candidate as deviceRef.
func (s *IngestService) resolveDeviceEnrichment(
	ctx context.Context,
	tenantId, orgId, sourceFamily string,
	candidates []ingestmod.DeviceIdentity,
) (*ingestmod.DeviceManagement, *ingestmod.DeviceIdentity) {
	if len(candidates) == 0 {
		return nil, nil
	}

	if s.deviceMgmtSvc != nil {
		for i := range candidates {
			enrichment := s.deviceMgmtSvc.Resolve(ctx, tenantId, orgId, sourceFamily, candidates[i].Type, candidates[i].ID)
			if enrichment != nil {
				ref := &candidates[i]
				if enrichment.DeviceId != "" {
					ref = &ingestmod.DeviceIdentity{Type: ref.Type, ID: enrichment.DeviceId}
				}
				return enrichment, ref
			}
		}
	}

	// No enrichment found — use first candidate as deviceRef
	return nil, &candidates[0]
}

// buildAutoUpsertHints extracts source-family-specific hints from a raw event body.
// Currently extracts "sn" (serial number) for AIBOX events.
func buildAutoUpsertHints(sourceFamily string, rawBody map[string]any) ingestmod.AutoUpsertHints {
	hints := ingestmod.AutoUpsertHints{}
	if sourceFamily == "AIBOX" {
		if v, ok := rawBody["sn"]; ok {
			if s, ok := v.(string); ok {
				hints.SerialNo = s
			}
		}
	}
	return hints
}

// buildMatchBag merges raw payload with canonical source.* fields for V2 template matching.
//
// Why: rawBody contains vendor-specific keys (e.g. AIBOX uses channelId for "device" 51).
// Users config templates against canonical concepts ("source.deviceId = 51") that they
// see in the UI. This bag exposes both worlds:
//
//   - Raw vendor fields stay at top level (backward compat with legacy templates that
//     still reference rawBody field names directly, e.g. {field: "deviceId", values: ["1"]})
//   - Canonical fields live under "source.*" (e.g. source.deviceId, source.deviceType,
//     source.deviceMgmtId, source.sn, source.zone)
//
// deviceRef is the winning DeviceIdentity from extractDeviceCandidates → resolveDeviceEnrichment
// (priority: cameraId > sensorId > faceId > channelId > deviceId). enrichment is the matched
// DeviceManagement record (nil if device is unknown to deviceManagement).
func buildMatchBag(
	rawBody map[string]any,
	deviceRef *ingestmod.DeviceIdentity,
	enrichment *ingestmod.DeviceManagement,
	sourceFamily string,
) map[string]any {
	bag := make(map[string]any, len(rawBody)+2)
	for k, v := range rawBody {
		bag[k] = v
	}
	// Expose raw payload under the "raw" namespace too, so conditions written
	// as "raw.<field>" resolve via the generic matcher's dotted-path lookup
	// without stage-specific prefix stripping (plan decision D6).
	bag["raw"] = rawBody

	source := map[string]any{
		"sourceFamily": sourceFamily,
	}
	if deviceRef != nil {
		source["deviceId"] = deviceRef.ID
		source["deviceType"] = deviceRef.Type
	}
	if enrichment != nil {
		if enrichment.DeviceMgmtId != "" {
			source["deviceMgmtId"] = enrichment.DeviceMgmtId
		}
		if enrichment.SerialNo != "" {
			source["sn"] = enrichment.SerialNo
		}
		if enrichment.Name != "" {
			source["name"] = enrichment.Name
		}
		if enrichment.Site != "" {
			source["site"] = enrichment.Site
		}
		if enrichment.Zone != "" {
			source["zone"] = enrichment.Zone
		}
	}
	bag["source"] = source
	return bag
}
