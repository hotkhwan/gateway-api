// internal/services/entitlementsvc/service.go
package entitlementsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/redis/go-redis/v9"
)

const (
	cacheKeyPrefix = "phibek:entitlement:"
	defaultTTL     = 5 * time.Minute
)

// ErrPayloadTooLarge is returned when the ingest payload exceeds the workspace limit.
var ErrPayloadTooLarge = errors.New("payload exceeds workspace entitlement limit")

// ErrQuotaExceeded is kept for existing callers in the ingest hot path that
// only need a boolean "can ingest" signal. GetForWorkspace no longer returns
// this error on cache miss because misses now synthesize a snapshot.
var ErrQuotaExceeded = errors.New("ingest not allowed: workspace entitlement quota exceeded or not found")

// SubscriptionResolver lets the service map a tenant's effective subscription
// onto RuntimeEntitlement fields regardless of deployment profile. In saasPublic
// the overlay reflects the tenant's paid plan; in appliance/enterprise it
// reflects any limits written by a platform-license activation.
type SubscriptionResolver interface {
	GetTenantEntitlementOverlay(ctx context.Context, tenantId string) (*TenantOverlay, error)
}

// WorkspaceTenantResolver returns the tenantId that owns a workspace. Used
// when callers (e.g. the ingest hot path) don't already know the tenantId so
// the service can still overlay subscription-derived limits on cache miss.
type WorkspaceTenantResolver interface {
	GetTenantIDForWorkspace(ctx context.Context, workspaceID string) (string, error)
}

// TenantOverlay carries the subset of subscription limits that overlap with
// RuntimeEntitlement. Zero values mean "do not overlay".
type TenantOverlay struct {
	PlanID              string
	MaxEventsPerSecond  int
	MaxPayloadBytes     int
	WebhookTargetsLimit int
}

// EntitlementService manages runtime entitlement enforcement for phibek.
// Source of truth is Redis (populated by entitlementcons for appliance and
// saasKlynx profiles); on cache miss the service synthesizes a product-neutral
// snapshot by combining the deployment profile catalog with the tenant's
// subscription overlay, warms the cache, and returns it.
type EntitlementService struct {
	redis     *redis.Client
	ttl       time.Duration
	profile   string
	catalog   *RuntimeEntitlementCatalog
	subLookup SubscriptionResolver
	wsLookup  WorkspaceTenantResolver
}

// New creates an EntitlementService backed by Redis. Resolvers are attached
// later via setters because their concrete services are built after
// entitlementsvc in the container composition root.
func New(redisClient *redis.Client) *EntitlementService {
	return &EntitlementService{
		redis:   redisClient,
		ttl:     defaultTTL,
		profile: os.Getenv("DEPLOYMENT_PROFILE"),
		catalog: NewRuntimeEntitlementCatalog(),
	}
}

// SetSubscriptionResolver wires the subscription service into the entitlement
// service. Safe to call once after both are constructed.
func (s *EntitlementService) SetSubscriptionResolver(r SubscriptionResolver) {
	s.subLookup = r
}

// SetWorkspaceTenantResolver wires the workspace→tenant lookup used when a
// caller passes an empty tenantId (ingest hot path, consumers).
func (s *EntitlementService) SetWorkspaceTenantResolver(r WorkspaceTenantResolver) {
	s.wsLookup = r
}

// GetWorkspaceEntitlement returns the RuntimeEntitlement for a workspace.
// Convenience wrapper that lets the service resolve tenantId internally. Use
// GetForWorkspace when the caller already has tenantId from auth locals.
func (s *EntitlementService) GetWorkspaceEntitlement(ctx context.Context, workspaceId string) (*RuntimeEntitlement, error) {
	return s.GetForWorkspace(ctx, workspaceId, "")
}

// GetForWorkspace is the tenant-aware variant. When tenantId is empty the
// service looks it up via WorkspaceTenantResolver so the subscription overlay
// still applies on the ingest hot path.
//
// Cache hit returns the cached snapshot. Cache miss synthesizes from the
// profile catalog, overlays the tenant's subscription limits (same logic in
// every profile so platform-license activations in appliance/enterprise take
// effect immediately), warms Redis, and returns the result. Only real Redis
// decode/read failures surface as errors.
func (s *EntitlementService) GetForWorkspace(ctx context.Context, workspaceId, tenantId string) (*RuntimeEntitlement, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/entitlementsvc",
		"entitlementsvc.GetForWorkspace",
		"entitlementsvc", "GetForWorkspace",
	)
	defer end()

	if workspaceId == "" {
		return nil, errors.New("workspaceId is required")
	}

	key := cacheKeyPrefix + workspaceId
	data, err := s.redis.Get(ctx, key).Bytes()
	if err == nil {
		var ent RuntimeEntitlement
		if unmarshalErr := json.Unmarshal(data, &ent); unmarshalErr != nil {
			log.Error().Err(unmarshalErr).Str("workspaceId", workspaceId).Msg("entitlement unmarshal failed")
			return nil, fmt.Errorf("entitlement unmarshal: %w", unmarshalErr)
		}
		return &ent, nil
	}

	if !errors.Is(err, redis.Nil) {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("redis get entitlement failed")
		return nil, fmt.Errorf("entitlement cache read: %w", err)
	}

	// Cache miss — synthesize.
	ent, synthErr := s.synthesize(ctx, workspaceId, tenantId)
	if synthErr != nil {
		log.Error().Err(synthErr).Str("workspaceId", workspaceId).Msg("entitlement synthesize failed")
		return nil, synthErr
	}

	if storeErr := s.writeCache(ctx, ent); storeErr != nil {
		// Non-fatal: caller still gets the synthesized snapshot; next request
		// will re-synthesize.
		log.Warn().Err(storeErr).Str("workspaceId", workspaceId).Msg("entitlement cache warm failed")
	}

	log.Info().
		Str("workspaceId", workspaceId).
		Str("planCode", ent.PlanCode).
		Str("profile", s.profile).
		Msg("entitlement synthesized on cache miss")

	return ent, nil
}

// synthesize builds a RuntimeEntitlement from the catalog plus the tenant
// subscription overlay. Always applies the overlay when a subscription
// resolver is configured — profile only selects the starting catalog entry,
// not whether overrides matter, so platform-license activations in
// appliance/enterprise still widen/narrow the returned snapshot.
func (s *EntitlementService) synthesize(ctx context.Context, workspaceId, tenantId string) (*RuntimeEntitlement, error) {
	base := s.catalog.ForProfile(s.profile)
	base.WorkspaceID = workspaceId

	tenantId = s.resolveTenantId(ctx, workspaceId, tenantId)
	if s.subLookup == nil || tenantId == "" {
		return &base, nil
	}

	overlay, err := s.subLookup.GetTenantEntitlementOverlay(ctx, tenantId)
	if err != nil {
		return nil, fmt.Errorf("subscription overlay lookup: %w", err)
	}
	if overlay == nil {
		return &base, nil
	}

	// In saasPublic a plan switch (e.g. freemium -> pro) changes the base
	// catalog entry entirely so derived fields like retention and source
	// families track the plan. In appliance/enterprise the profile default
	// is already the widest tier; we only narrow via the overlay fields, so
	// leaving the base untouched preserves unlimited semantics until the
	// overlay explicitly sets a stricter limit.
	if s.profile == "saasPublic" && overlay.PlanID != "" {
		if plan := s.catalog.Default(overlay.PlanID); plan.PlanCode != "" {
			plan.WorkspaceID = workspaceId
			base = plan
		}
	}
	if overlay.MaxEventsPerSecond > 0 {
		base.MaxEventsPerSecond = overlay.MaxEventsPerSecond
	}
	if overlay.MaxPayloadBytes > 0 {
		base.MaxPayloadBytes = overlay.MaxPayloadBytes
	}
	if overlay.WebhookTargetsLimit > 0 {
		base.WebhookTargetsLimit = overlay.WebhookTargetsLimit
	}

	// Clamp payload to Kafka-safe max so a tenant override never produces a
	// RuntimeEntitlement the ingest hot path can't honor.
	if kafkaSafe := config.GetKafkaSafeMaxPayloadBytes(); kafkaSafe > 0 {
		if int64(base.MaxPayloadBytes) > kafkaSafe {
			base.MaxPayloadBytes = int(kafkaSafe)
		}
	}

	return &base, nil
}

// resolveTenantId returns the caller-provided tenantId when present, otherwise
// looks it up from the workspace record. Any lookup error is swallowed: on
// ingest hot path we prefer "fall back to catalog defaults" over "block the
// request" when the workspace directory is briefly unreachable.
func (s *EntitlementService) resolveTenantId(ctx context.Context, workspaceId, tenantId string) string {
	if tenantId != "" || s.wsLookup == nil || workspaceId == "" {
		return tenantId
	}
	resolved, err := s.wsLookup.GetTenantIDForWorkspace(ctx, workspaceId)
	if err != nil {
		log := logger.FromCtx(ctx, "entitlementsvc", "resolveTenantId")
		log.Warn().Err(err).Str("workspaceId", workspaceId).Msg("tenant lookup failed — falling back to catalog-only synthesis")
		return ""
	}
	return resolved
}

func (s *EntitlementService) writeCache(ctx context.Context, ent *RuntimeEntitlement) error {
	if ent == nil || ent.WorkspaceID == "" {
		return errors.New("entitlement missing workspaceId")
	}
	data, err := json.Marshal(ent)
	if err != nil {
		return fmt.Errorf("entitlement marshal: %w", err)
	}
	key := cacheKeyPrefix + ent.WorkspaceID
	return s.redis.Set(ctx, key, data, s.ttl).Err()
}

// CheckIngestAllowed verifies that the workspace entitlement permits ingesting
// a payload of the given byte size. Cache miss no longer maps to
// ErrQuotaExceeded because GetForWorkspace now synthesizes a snapshot using
// the workspace→tenant resolver so the tenant's real subscription still
// applies after TTL expiry.
func (s *EntitlementService) CheckIngestAllowed(ctx context.Context, workspaceId string, payloadBytes int) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/entitlementsvc",
		"entitlementsvc.CheckIngestAllowed",
		"entitlementsvc", "CheckIngestAllowed",
	)
	defer end()

	ent, err := s.GetForWorkspace(ctx, workspaceId, "")
	if err != nil {
		return err
	}

	if ent.MaxPayloadBytes > 0 && payloadBytes > ent.MaxPayloadBytes {
		log.Warn().
			Str("workspaceId", workspaceId).
			Int("payloadBytes", payloadBytes).
			Int("maxPayloadBytes", ent.MaxPayloadBytes).
			Msg("payload too large for workspace entitlement")
		return ErrPayloadTooLarge
	}

	return nil
}

// StoreEntitlement writes a RuntimeEntitlement snapshot to Redis.
// Called by entitlementcons when a klynx.entitlement.snapshot.v1 message is received.
func (s *EntitlementService) StoreEntitlement(ctx context.Context, ent *RuntimeEntitlement) error {
	log := logger.FromCtx(ctx, "entitlementsvc", "StoreEntitlement")

	if err := s.writeCache(ctx, ent); err != nil {
		log.Error().Err(err).Str("workspaceId", ent.WorkspaceID).Msg("redis set entitlement failed")
		return fmt.Errorf("entitlement cache write: %w", err)
	}

	log.Info().
		Str("workspaceId", ent.WorkspaceID).
		Str("planCode", ent.PlanCode).
		Msg("entitlement snapshot cached")

	return nil
}

// InvalidateWorkspace drops the cached RuntimeEntitlement for a workspace so
// the next read re-synthesizes or re-fetches. Used by platform license
// activation so a fresh entitlement reflects the newly applied license
// immediately.
func (s *EntitlementService) InvalidateWorkspace(ctx context.Context, workspaceId string) error {
	if workspaceId == "" {
		return nil
	}
	return s.redis.Del(ctx, cacheKeyPrefix+workspaceId).Err()
}

// InvalidateWorkspaces drops caches for a list of workspaces in one Redis
// round-trip. Used after platform-license activation to refresh every
// workspace owned by the activating tenant.
func (s *EntitlementService) InvalidateWorkspaces(ctx context.Context, workspaceIds []string) error {
	if len(workspaceIds) == 0 {
		return nil
	}
	keys := make([]string, 0, len(workspaceIds))
	for _, id := range workspaceIds {
		if id == "" {
			continue
		}
		keys = append(keys, cacheKeyPrefix+id)
	}
	if len(keys) == 0 {
		return nil
	}
	return s.redis.Del(ctx, keys...).Err()
}
