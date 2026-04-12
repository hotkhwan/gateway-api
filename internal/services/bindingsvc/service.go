// internal/services/bindingsvc/service.go
package bindingsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/bindingrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
)

// bindingRepoI is the repo subset used by BindingService.
// *bindingrepo.BindingRepo satisfies this interface.
type bindingRepoI interface {
	Insert(ctx context.Context, b *ingestmod.TemplateDeliveryBinding) error
	FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.TemplateDeliveryBinding, error)
	FindByWorkspaceAndStage(ctx context.Context, workspaceId, stage string) ([]ingestmod.TemplateDeliveryBinding, error)
	FindAllByStage(ctx context.Context, stage string) ([]ingestmod.TemplateDeliveryBinding, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error)
	Update(ctx context.Context, workspaceId, id string, fields bson.M) error
	Delete(ctx context.Context, workspaceId, id string) error
}

// redisI is the Redis subset used by BindingService.
// *redis.Client satisfies this interface.
type redisI interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// realtimeCacheKey returns the Redis key for realtime bindings of a workspace.
// Pattern: realtime_bindings:{workspaceId}
func realtimeCacheKey(workspaceId string) string {
	return "realtime_bindings:" + workspaceId
}

// ============================================================
// Input types
// ============================================================

type CreateBindingInput struct {
	WorkspaceID       string
	TemplateID        string
	TargetID          string
	DispatchStage     string         // "normalize" | "realtime"
	MatchFields       map[string]any
	MessageTemplateID string
	Enabled           bool
}

type UpdateBindingInput struct {
	WorkspaceID       string
	ID                string
	DispatchStage     *string
	MatchFields       map[string]any
	MessageTemplateID *string
	Enabled           *bool
}

// ============================================================
// BindingService
// ============================================================

type BindingService struct {
	repo  bindingRepoI
	redis redisI
}

func NewBindingService(repo bindingRepoI, redis redisI) *BindingService {
	if repo == nil {
		panic("BindingService: repo required")
	}
	if redis == nil {
		panic("BindingService: redis required")
	}
	return &BindingService{repo: repo, redis: redis}
}

// ============================================================
// Create
// ============================================================

func (s *BindingService) Create(ctx context.Context, input CreateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	log := logger.FromCtx(ctx, "bindingsvc", "BindingService.Create")

	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.DispatchStage = strings.TrimSpace(input.DispatchStage)

	if input.WorkspaceID == "" || input.TemplateID == "" || input.TargetID == "" {
		return nil, ErrBadRequest
	}
	if input.DispatchStage != ingestmod.DispatchStageNormalize && input.DispatchStage != ingestmod.DispatchStageRealtime {
		return nil, ErrBadRequest
	}

	b := &ingestmod.TemplateDeliveryBinding{
		WorkspaceID:       input.WorkspaceID,
		TemplateID:        input.TemplateID,
		TargetID:          input.TargetID,
		DispatchStage:     input.DispatchStage,
		MatchFields:       input.MatchFields,
		MessageTemplateID: input.MessageTemplateID,
		Enabled:           input.Enabled,
	}

	if err := s.repo.Insert(ctx, b); err != nil {
		log.Error().Err(err).Msg("binding insert failed")
		return nil, err
	}

	// Warm or invalidate realtime cache
	if input.DispatchStage == ingestmod.DispatchStageRealtime {
		s.warmRealtimeCache(ctx, input.WorkspaceID)
	}

	log.Info().Str("bindingId", b.ID).Str("workspaceId", input.WorkspaceID).Msg("binding created")
	return b, nil
}

// ============================================================
// List
// ============================================================

func (s *BindingService) List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error) {
	if workspaceId == "" {
		return nil, nil, ErrBadRequest
	}
	return s.repo.List(ctx, workspaceId, page, perPage)
}

// ============================================================
// GetOne
// ============================================================

func (s *BindingService) GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.TemplateDeliveryBinding, error) {
	if workspaceId == "" || id == "" {
		return nil, ErrBadRequest
	}
	b, err := s.repo.FindByID(ctx, workspaceId, id)
	if err != nil {
		if errors.Is(err, bindingrepo.ErrBindingNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return b, nil
}

// ============================================================
// Update
// ============================================================

func (s *BindingService) Update(ctx context.Context, input UpdateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	log := logger.FromCtx(ctx, "bindingsvc", "BindingService.Update")

	if input.WorkspaceID == "" || input.ID == "" {
		return nil, ErrBadRequest
	}

	existing, err := s.repo.FindByID(ctx, input.WorkspaceID, input.ID)
	if err != nil {
		if errors.Is(err, bindingrepo.ErrBindingNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	fields := bson.M{}

	if input.DispatchStage != nil {
		stage := strings.TrimSpace(*input.DispatchStage)
		if stage != ingestmod.DispatchStageNormalize && stage != ingestmod.DispatchStageRealtime {
			return nil, ErrBadRequest
		}
		fields["dispatchStage"] = stage
	}
	if input.MatchFields != nil {
		fields["matchFields"] = input.MatchFields
	}
	if input.MessageTemplateID != nil {
		fields["messageTemplateId"] = *input.MessageTemplateID
	}
	if input.Enabled != nil {
		fields["enabled"] = *input.Enabled
	}

	if len(fields) == 0 {
		return nil, ErrBadRequest
	}

	if err := s.repo.Update(ctx, input.WorkspaceID, input.ID, fields); err != nil {
		return nil, err
	}

	// Invalidate realtime cache if this binding affects realtime stage
	oldStage := existing.DispatchStage
	newStage := oldStage
	if input.DispatchStage != nil {
		newStage = *input.DispatchStage
	}
	if oldStage == ingestmod.DispatchStageRealtime || newStage == ingestmod.DispatchStageRealtime {
		s.warmRealtimeCache(ctx, input.WorkspaceID)
	}

	log.Info().Str("bindingId", input.ID).Str("workspaceId", input.WorkspaceID).Msg("binding updated")
	return s.repo.FindByID(ctx, input.WorkspaceID, input.ID)
}

// ============================================================
// Delete
// ============================================================

func (s *BindingService) Delete(ctx context.Context, workspaceId, id string) error {
	log := logger.FromCtx(ctx, "bindingsvc", "BindingService.Delete")

	if workspaceId == "" || id == "" {
		return ErrBadRequest
	}

	existing, err := s.repo.FindByID(ctx, workspaceId, id)
	if err != nil {
		if errors.Is(err, bindingrepo.ErrBindingNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.repo.Delete(ctx, workspaceId, id); err != nil {
		return err
	}

	// Invalidate realtime cache if this was a realtime binding
	if existing.DispatchStage == ingestmod.DispatchStageRealtime {
		s.invalidateRealtimeCache(ctx, workspaceId)
	}

	log.Info().Str("bindingId", id).Str("workspaceId", workspaceId).Msg("binding deleted")
	return nil
}

// ============================================================
// Redis cache management
// ============================================================

// GetRealtimeBindings returns realtime bindings for a workspace from Redis.
// Returns nil (not found) if the key is missing — callers should skip Path A on MISS.
func (s *BindingService) GetRealtimeBindings(ctx context.Context, workspaceId string) ([]ingestmod.TemplateDeliveryBinding, bool) {
	raw, err := s.redis.Get(ctx, realtimeCacheKey(workspaceId)).Bytes()
	if err != nil {
		return nil, false
	}
	var bindings []ingestmod.TemplateDeliveryBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return nil, false
	}
	return bindings, true
}

// warmRealtimeCache reloads realtime bindings from DB and stores in Redis (no TTL — invalidate on mutation).
func (s *BindingService) warmRealtimeCache(ctx context.Context, workspaceId string) {
	log := logger.FromCtx(ctx, "bindingsvc", "warmRealtimeCache")
	bindings, err := s.repo.FindByWorkspaceAndStage(ctx, workspaceId, ingestmod.DispatchStageRealtime)
	if err != nil {
		log.Warn().Str("workspaceId", workspaceId).Err(err).Msg("[bindingsvc] reload realtime bindings failed")
		return
	}
	data, err := json.Marshal(bindings)
	if err != nil {
		return
	}
	// No TTL — invalidate on mutations. Key persists until explicit delete.
	if err := s.redis.Set(ctx, realtimeCacheKey(workspaceId), data, 0).Err(); err != nil {
		log.Warn().Str("workspaceId", workspaceId).Err(err).Msg("[bindingsvc] set realtime cache failed")
	}
}

// InvalidateRealtimeCache removes the realtime bindings cache for a workspace.
// Call on workspace delete or when all realtime bindings are deleted.
func (s *BindingService) InvalidateRealtimeCache(ctx context.Context, workspaceId string) {
	s.invalidateRealtimeCache(ctx, workspaceId)
}

func (s *BindingService) invalidateRealtimeCache(ctx context.Context, workspaceId string) {
	log := logger.FromCtx(ctx, "bindingsvc", "invalidateRealtimeCache")
	if err := s.redis.Del(ctx, realtimeCacheKey(workspaceId)).Err(); err != nil {
		log.Warn().Str("workspaceId", workspaceId).Err(err).Msg("[bindingsvc] del realtime cache failed")
	}
}

// WarmRealtimeCacheOnStartup warms the realtime binding cache for all workspaces
// that have at least one realtime binding. Call during app bootstrap.
// Non-fatal: logs warning on error, never returns error.
func (s *BindingService) WarmRealtimeCacheOnStartup(ctx context.Context) {
	log := logger.FromCtx(ctx, "bindingsvc", "WarmRealtimeCacheOnStartup")
	// Find all realtime bindings across all workspaces
	bindings, err := s.repo.FindAllByStage(ctx, ingestmod.DispatchStageRealtime)
	if err != nil {
		log.Warn().Err(err).Msg("[bindingsvc] startup cache warm: list failed (non-fatal)")
		return
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		if !seen[b.WorkspaceID] {
			seen[b.WorkspaceID] = true
			s.warmRealtimeCache(ctx, b.WorkspaceID)
		}
	}
	log.Info().Int("workspaces", len(seen)).Msg("[bindingsvc] startup realtime cache warmed")
}

// MatchesFields returns true when all matchFields key=value conditions are satisfied by the payload.
// An empty/nil matchFields map passes all payloads (wildcard).
func MatchesFields(payload map[string]any, matchFields map[string]any) bool {
	for k, v := range matchFields {
		actual, ok := payload[k]
		if !ok {
			return false
		}
		// Compare as strings for simplicity (the plan uses JSON-like values)
		if toString(actual) != toString(v) {
			return false
		}
	}
	return true
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		b, _ := json.Marshal(v)
		return strings.Trim(string(b), `"`)
	}
}

// ============================================================
// GetNormalizeBindings returns normalize-stage bindings from DB (no Redis cache).
// Used by normalizedcons for post-normalize webhook dispatch.
// ============================================================

func (s *BindingService) GetNormalizeBindings(ctx context.Context, workspaceId string) ([]ingestmod.TemplateDeliveryBinding, error) {
	return s.repo.FindByWorkspaceAndStage(ctx, workspaceId, ingestmod.DispatchStageNormalize)
}

// ============================================================
// WarmRealtimeForWorkspace exposes warmRealtimeCache for container startup.
// ============================================================

func (s *BindingService) WarmRealtimeForWorkspace(ctx context.Context, workspaceId string) {
	s.warmRealtimeCache(ctx, workspaceId)
}

// GetRealtimeBindingsFromDB returns realtime bindings directly from DB (bypass cache).
func (s *BindingService) GetRealtimeBindingsFromDB(ctx context.Context, workspaceId string) ([]ingestmod.TemplateDeliveryBinding, error) {
	return s.repo.FindByWorkspaceAndStage(ctx, workspaceId, ingestmod.DispatchStageRealtime)
}

