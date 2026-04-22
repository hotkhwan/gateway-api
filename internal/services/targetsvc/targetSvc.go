// internal/services/targetsvc/targetSvc.go
package targetsvc

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

// TargetRepoI is the repo subset used by TargetService.
// *targetrepo.TargetRepo satisfies this interface.
type TargetRepoI interface {
	ExistsByNameInOrg(ctx context.Context, tenantId, workspaceId, name, excludeID string) (bool, error)
	Insert(ctx context.Context, t *authzmod.DeliveryTarget) error
	FindByIDAndOrg(ctx context.Context, targetId, tenantId, workspaceId string) (*authzmod.DeliveryTarget, error)
	List(ctx context.Context, tenantId, workspaceId, search string, page, perPage int, sortField, sortOrder string) ([]authzmod.DeliveryTarget, int64, error)
	Update(ctx context.Context, targetId, tenantId, workspaceId string, fields bson.M) error
	Delete(ctx context.Context, targetId, tenantId, workspaceId string) error
	CountByTypeAndOrg(ctx context.Context, tenantId, workspaceId, targetType string) (int64, error)
	CountMessageChannelsByOrg(ctx context.Context, tenantId, workspaceId string) (int64, error)
}

// TemplateUsageRepoI reports which mapping templates still reference a given
// delivery target. Scoped to a workspace so deletion of a workspace target
// cannot be blocked by a template in another workspace.
type TemplateUsageRepoI interface {
	FindUsageByTargetId(ctx context.Context, workspaceId, targetId string) ([]ingestrepo.TemplateUsageRef, error)
}

// ============================================================
// Input types
// ============================================================

type CreateTargetInput struct {
	TenantId        string
	WorkspaceId     string
	UserId          string
	Name            string
	Type            string // webhook|line|telegram|discord
	Mode            string // "klynx" = system routing marker; absent for regular targets
	Enabled         bool
	Config          authzmod.TargetConfig
	IsPlatformAdmin bool
}

type UpdateTargetInput struct {
	TenantId        string
	WorkspaceId     string
	TargetId        string
	UserId          string
	Name            *string
	Enabled         *bool
	Config          *authzmod.TargetConfig
	IsPlatformAdmin bool
}

type ListTargetInput struct {
	TenantId    string
	WorkspaceId string
	Search      string
	Page        int
	PerPage     int
	SortField   string
	SortOrder   string
}

// ============================================================
// TargetService
// ============================================================

type TargetService struct {
	repo        TargetRepoI
	tmplRepo    TemplateUsageRepoI
	authzClient authzgw.Client
	subSvc      *subscriptionsvc.SubscriptionService
}

func NewTargetService(repo TargetRepoI, tmplRepo TemplateUsageRepoI, authzClient authzgw.Client, subSvc *subscriptionsvc.SubscriptionService) *TargetService {
	if repo == nil || tmplRepo == nil || authzClient == nil {
		panic("TargetService: repo, tmplRepo and authzClient required")
	}
	return &TargetService{repo: repo, tmplRepo: tmplRepo, authzClient: authzClient, subSvc: subSvc}
}

// validTargetTypes ตรวจ type ก่อน insert
var validTargetTypes = map[string]bool{
	authzmod.TargetTypeWebhook:  true,
	authzmod.TargetTypeLine:     true,
	authzmod.TargetTypeTelegram: true,
	authzmod.TargetTypeDiscord:  true,
}

// normalizeConfigForType trims whitespace from per-type identifier/secret fields
// before validation and storage. Without this, a value like " @phibek_channel"
// passes validation but Telegram rejects it at delivery time as "chat not found".
func normalizeConfigForType(targetType string, cfg *authzmod.TargetConfig) {
	if cfg == nil {
		return
	}
	switch targetType {
	case authzmod.TargetTypeTelegram:
		cfg.BotToken = strings.TrimSpace(cfg.BotToken)
		cfg.ChatId = strings.TrimSpace(cfg.ChatId)
	case authzmod.TargetTypeLine:
		cfg.ChannelAccessToken = strings.TrimSpace(cfg.ChannelAccessToken)
		cfg.ChannelAccessTokenRef = strings.TrimSpace(cfg.ChannelAccessTokenRef)
		for i, r := range cfg.To {
			cfg.To[i] = strings.TrimSpace(r)
		}
	case authzmod.TargetTypeWebhook, authzmod.TargetTypeDiscord:
		cfg.URL = strings.TrimSpace(cfg.URL)
		cfg.SigningSecret = strings.TrimSpace(cfg.SigningSecret)
	}
}

// validateConfigForType enforces per-type required config fields so empty
// secrets cannot slip through and surface only at delivery time.
// Skipped for mode=klynx — system routing markers do not carry creds.
func validateConfigForType(targetType, mode string, cfg authzmod.TargetConfig) error {
	if mode == "klynx" {
		return nil
	}
	switch targetType {
	case authzmod.TargetTypeTelegram:
		if strings.TrimSpace(cfg.BotToken) == "" {
			return ErrMissingBotToken
		}
		if strings.TrimSpace(cfg.ChatId) == "" {
			return ErrMissingChatId
		}
	case authzmod.TargetTypeLine:
		if strings.TrimSpace(cfg.ChannelAccessToken) == "" && strings.TrimSpace(cfg.ChannelAccessTokenRef) == "" {
			return ErrMissingChannelToken
		}
		if len(cfg.To) == 0 {
			return ErrMissingRecipients
		}
	case authzmod.TargetTypeWebhook, authzmod.TargetTypeDiscord:
		if strings.TrimSpace(cfg.URL) == "" {
			return ErrMissingURL
		}
	}
	return nil
}

// mergeConfigPreservingSecrets produces the effective config for an Update
// by keeping every existing secret/ref field whose incoming value is empty.
// This prevents accidental wipes when a UI shows a masked secret field and
// does not re-send it on save.
func mergeConfigPreservingSecrets(existing, incoming authzmod.TargetConfig) authzmod.TargetConfig {
	merged := incoming
	if strings.TrimSpace(merged.BotToken) == "" {
		merged.BotToken = existing.BotToken
	}
	if strings.TrimSpace(merged.ChannelAccessToken) == "" {
		merged.ChannelAccessToken = existing.ChannelAccessToken
	}
	if strings.TrimSpace(merged.ChannelAccessTokenRef) == "" {
		merged.ChannelAccessTokenRef = existing.ChannelAccessTokenRef
	}
	if strings.TrimSpace(merged.SigningSecret) == "" {
		merged.SigningSecret = existing.SigningSecret
	}
	return merged
}

// checkAdmin ตรวจว่า userId เป็น admin ของ workspace ใน Permify
// bypass=true สำหรับ platform admin (JWT role=administrator) ที่ผ่าน middleware แล้ว
func (s *TargetService) checkAdmin(ctx context.Context, tenantId, workspaceId, userId string, bypass bool) error {
	if bypass {
		return nil
	}
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", workspaceId, "manage", "user", userId,
	)
	if err != nil || !allowed {
		return ErrForbidden
	}
	return nil
}

// ============================================================
// Create
// ============================================================

func (s *TargetService) Create(ctx context.Context, input CreateTargetInput) (*authzmod.DeliveryTarget, error) {
	log := logger.FromCtx(ctx, "targetsvc", "Target.Create")

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || input.WorkspaceId == "" || input.TenantId == "" || input.UserId == "" {
		return nil, ErrBadRequest
	}
	if !validTargetTypes[input.Type] {
		return nil, ErrBadRequest
	}

	// mode=klynx validation: system routing marker — no url/hmac, appliance-only
	if input.Mode == "klynx" {
		if input.Config.URL != "" {
			return nil, ErrKlynxModeWithURL
		}
		if input.Config.SigningSecret != "" || input.Config.SigningEnabled {
			return nil, ErrKlynxModeWithHMAC
		}
		if os.Getenv("DEPLOYMENT_PROFILE") == "saasPublic" {
			return nil, ErrKlynxModeInSaasPublic
		}
	}

	normalizeConfigForType(input.Type, &input.Config)
	if err := validateConfigForType(input.Type, input.Mode, input.Config); err != nil {
		return nil, err
	}

	if err := s.checkAdmin(ctx, input.TenantId, input.WorkspaceId, input.UserId, input.IsPlatformAdmin); err != nil {
		return nil, err
	}

	// Quota check — enforce plan limits before creating
	if s.subSvc != nil {
		if err := s.subSvc.ValidateDeliveryTargetQuota(ctx, input.TenantId, input.WorkspaceId, input.Type, s.repo); err != nil {
			if errors.Is(err, subscriptionsvc.ErrDeliveryQuotaExceeded) {
				return nil, ErrPlanLimitExceeded
			}
			return nil, err
		}
	}

	exists, err := s.repo.ExistsByNameInOrg(ctx, input.TenantId, input.WorkspaceId, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrConflict
	}

	target := &authzmod.DeliveryTarget{
		TenantId:    input.TenantId,
		WorkspaceId: input.WorkspaceId,
		Name:        input.Name,
		Type:        input.Type,
		Mode:        input.Mode,
		Enabled:     input.Enabled,
		Config:      input.Config,
		CreatedBy:   input.UserId,
	}

	if err := s.repo.Insert(ctx, target); err != nil {
		return nil, err
	}

	// Permify: target#parentOrg@organization:workspaceId
	tuples := []map[string]interface{}{
		{
			"entity":   map[string]string{"type": "target", "id": target.TargetId},
			"relation": "parentOrg",
			"subject":  map[string]string{"type": "organization", "id": input.WorkspaceId},
		},
	}
	if err := s.authzClient.WriteTuples(ctx, input.TenantId, tuples); err != nil {
		log.Error().Err(err).Str("targetId", target.TargetId).Msg("permify write failed (non-fatal)")
	}

	log.Info().Str("targetId", target.TargetId).Str("type", input.Type).Msg("delivery target created")
	return target, nil
}

// ============================================================
// List
// ============================================================

func (s *TargetService) List(ctx context.Context, input ListTargetInput) ([]authzmod.DeliveryTarget, int64, error) {
	if input.TenantId == "" || input.WorkspaceId == "" {
		return nil, 0, ErrBadRequest
	}

	page := input.Page
	if page < 1 {
		page = 1
	}
	perPage := input.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	return s.repo.List(ctx, input.TenantId, input.WorkspaceId, input.Search, page, perPage, input.SortField, input.SortOrder)
}

// ============================================================
// GetOne
// ============================================================

func (s *TargetService) GetOne(ctx context.Context, tenantId, workspaceId, userId, targetId string, isAdmin bool) (*authzmod.DeliveryTarget, error) {
	if tenantId == "" || workspaceId == "" || targetId == "" {
		return nil, ErrBadRequest
	}

	if err := s.checkAdmin(ctx, tenantId, workspaceId, userId, isAdmin); err != nil {
		return nil, err
	}

	target, err := s.repo.FindByIDAndOrg(ctx, targetId, tenantId, workspaceId)
	if err != nil {
		if errors.Is(err, targetrepo.ErrTargetNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return target, nil
}

// ============================================================
// Update
// ============================================================

func (s *TargetService) Update(ctx context.Context, input UpdateTargetInput) (*authzmod.DeliveryTarget, error) {
	log := logger.FromCtx(ctx, "targetsvc", "Target.Update")

	if input.TenantId == "" || input.WorkspaceId == "" || input.TargetId == "" {
		return nil, ErrBadRequest
	}

	if err := s.checkAdmin(ctx, input.TenantId, input.WorkspaceId, input.UserId, input.IsPlatformAdmin); err != nil {
		return nil, err
	}

	existing, err := s.repo.FindByIDAndOrg(ctx, input.TargetId, input.TenantId, input.WorkspaceId)
	if err != nil {
		if errors.Is(err, targetrepo.ErrTargetNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	fields := bson.M{}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrBadRequest
		}
		exists, err := s.repo.ExistsByNameInOrg(ctx, input.TenantId, input.WorkspaceId, name, input.TargetId)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrConflict
		}
		fields["name"] = name
	}

	if input.Enabled != nil {
		fields["enabled"] = *input.Enabled
	}

	if input.Config != nil {
		normalizeConfigForType(existing.Type, input.Config)
		merged := mergeConfigPreservingSecrets(existing.Config, *input.Config)
		normalizeConfigForType(existing.Type, &merged)
		if err := validateConfigForType(existing.Type, existing.Mode, merged); err != nil {
			return nil, err
		}
		fields["config"] = merged
	}

	if len(fields) == 0 {
		return nil, ErrBadRequest
	}

	if err := s.repo.Update(ctx, input.TargetId, input.TenantId, input.WorkspaceId, fields); err != nil {
		return nil, err
	}

	log.Info().Str("targetId", input.TargetId).Msg("delivery target updated")
	return s.repo.FindByIDAndOrg(ctx, input.TargetId, input.TenantId, input.WorkspaceId)
}

// ============================================================
// Delete
// ============================================================

func (s *TargetService) Delete(ctx context.Context, tenantId, workspaceId, userId, targetId string, isAdmin bool) error {
	log := logger.FromCtx(ctx, "targetsvc", "Target.Delete")

	if tenantId == "" || workspaceId == "" || targetId == "" {
		return ErrBadRequest
	}

	if err := s.checkAdmin(ctx, tenantId, workspaceId, userId, isAdmin); err != nil {
		return err
	}

	if _, err := s.repo.FindByIDAndOrg(ctx, targetId, tenantId, workspaceId); err != nil {
		if errors.Is(err, targetrepo.ErrTargetNotFound) {
			return ErrNotFound
		}
		return err
	}

	usage, err := s.tmplRepo.FindUsageByTargetId(ctx, workspaceId, targetId)
	if err != nil {
		return err
	}
	if len(usage) > 0 {
		return &TargetInUseError{Templates: usage}
	}

	_ = s.authzClient.DeleteEntityRelationships(ctx, tenantId, "target", targetId)

	if err := s.repo.Delete(ctx, targetId, tenantId, workspaceId); err != nil {
		return err
	}

	log.Info().Str("targetId", targetId).Str("workspaceId", workspaceId).Msg("delivery target deleted")
	return nil
}

// ============================================================
// ToggleEnabled
// ============================================================

func (s *TargetService) ToggleEnabled(ctx context.Context, tenantId, workspaceId, userId, targetId string, enabled bool, isAdmin bool) error {
	if err := s.checkAdmin(ctx, tenantId, workspaceId, userId, isAdmin); err != nil {
		return err
	}
	return s.repo.Update(ctx, targetId, tenantId, workspaceId, bson.M{"enabled": enabled, "updatedAt": time.Now().UTC()})
}
