// internal/services/entitlementsvc/service.go
package entitlementsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// ErrQuotaExceeded is returned when the workspace entitlement is not found or ingest is not allowed.
var ErrQuotaExceeded = errors.New("ingest not allowed: workspace entitlement quota exceeded or not found")

// EntitlementService manages runtime entitlement enforcement for phibek.
// It reads entitlement snapshots from Redis (populated by entitlementcons)
// and gates ingest requests without querying klynx-api on every event.
type EntitlementService struct {
	redis *redis.Client
	ttl   time.Duration
}

// New creates an EntitlementService backed by Redis.
func New(redisClient *redis.Client) *EntitlementService {
	return &EntitlementService{
		redis: redisClient,
		ttl:   defaultTTL,
	}
}

// GetWorkspaceEntitlement returns the cached RuntimeEntitlement for a workspace.
// Returns ErrQuotaExceeded if the entitlement is not cached (workspace unknown or sync pending).
func (s *EntitlementService) GetWorkspaceEntitlement(ctx context.Context, workspaceId string) (*RuntimeEntitlement, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/entitlementsvc",
		"entitlementsvc.GetWorkspaceEntitlement",
		"entitlementsvc", "GetWorkspaceEntitlement",
	)
	defer end()

	key := cacheKeyPrefix + workspaceId
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.Warn().Str("workspaceId", workspaceId).Msg("entitlement not cached")
			return nil, ErrQuotaExceeded
		}
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("redis get entitlement failed")
		return nil, fmt.Errorf("entitlement cache read: %w", err)
	}

	var ent RuntimeEntitlement
	if err := json.Unmarshal(data, &ent); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("entitlement unmarshal failed")
		return nil, fmt.Errorf("entitlement unmarshal: %w", err)
	}

	return &ent, nil
}

// CheckIngestAllowed verifies that the workspace entitlement permits ingesting
// a payload of the given byte size.
func (s *EntitlementService) CheckIngestAllowed(ctx context.Context, workspaceId string, payloadBytes int) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/entitlementsvc",
		"entitlementsvc.CheckIngestAllowed",
		"entitlementsvc", "CheckIngestAllowed",
	)
	defer end()

	ent, err := s.GetWorkspaceEntitlement(ctx, workspaceId)
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

	data, err := json.Marshal(ent)
	if err != nil {
		return fmt.Errorf("entitlement marshal: %w", err)
	}

	key := cacheKeyPrefix + ent.WorkspaceID
	if err := s.redis.Set(ctx, key, data, s.ttl).Err(); err != nil {
		log.Error().Err(err).Str("workspaceId", ent.WorkspaceID).Msg("redis set entitlement failed")
		return fmt.Errorf("entitlement cache write: %w", err)
	}

	log.Info().
		Str("workspaceId", ent.WorkspaceID).
		Str("planCode", ent.PlanCode).
		Msg("entitlement snapshot cached")

	return nil
}
