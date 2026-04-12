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
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

// TargetRepoI is the repo subset used by TargetService.
// *targetrepo.TargetRepo satisfies this interface.
type TargetRepoI interface {
	ExistsByNameInOrg(ctx context.Context, tenantId, orgId, name, excludeID string) (bool, error)
	Insert(ctx context.Context, t *authzmod.DeliveryTarget) error
	FindByIDAndOrg(ctx context.Context, targetId, tenantId, orgId string) (*authzmod.DeliveryTarget, error)
	List(ctx context.Context, tenantId, orgId, search string, page, perPage int, sortField, sortOrder string) ([]authzmod.DeliveryTarget, int64, error)
	Update(ctx context.Context, targetId, tenantId, orgId string, fields bson.M) error
	Delete(ctx context.Context, targetId, tenantId, orgId string) error
	CountByTypeAndOrg(ctx context.Context, tenantId, orgId, targetType string) (int64, error)
	CountMessageChannelsByOrg(ctx context.Context, tenantId, orgId string) (int64, error)
}

// ============================================================
// Input types
// ============================================================

type CreateTargetInput struct {
	TenantId string
	OrgId    string
	UserId   string
	Name     string
	Type     string // webhook|line|telegram|discord
	Mode     string // "klynx" = system routing marker; absent for regular targets
	Enabled  bool
	Config   authzmod.TargetConfig
}

type UpdateTargetInput struct {
	TenantId string
	OrgId    string
	TargetId string
	UserId   string
	Name     *string
	Enabled  *bool
	Config   *authzmod.TargetConfig
}

type ListTargetInput struct {
	TenantId  string
	OrgId     string
	Search    string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}

// ============================================================
// TargetService
// ============================================================

type TargetService struct {
	repo        TargetRepoI
	authzClient authzgw.Client
	subSvc      *subscriptionsvc.SubscriptionService
}

func NewTargetService(repo TargetRepoI, authzClient authzgw.Client, subSvc *subscriptionsvc.SubscriptionService) *TargetService {
	if repo == nil || authzClient == nil {
		panic("TargetService: repo and authzClient required")
	}
	return &TargetService{repo: repo, authzClient: authzClient, subSvc: subSvc}
}

// validTargetTypes ตรวจ type ก่อน insert
var validTargetTypes = map[string]bool{
	authzmod.TargetTypeWebhook:  true,
	authzmod.TargetTypeLine:     true,
	authzmod.TargetTypeTelegram: true,
	authzmod.TargetTypeDiscord:  true,
}

// checkAdmin ตรวจว่า userId เป็น admin ของ orgId ใน Permify
func (s *TargetService) checkAdmin(ctx context.Context, tenantId, orgId, userId string) error {
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", orgId, "manage", "user", userId,
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
	if input.Name == "" || input.OrgId == "" || input.TenantId == "" || input.UserId == "" {
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

	if err := s.checkAdmin(ctx, input.TenantId, input.OrgId, input.UserId); err != nil {
		return nil, err
	}

	// Quota check — enforce plan limits before creating
	if s.subSvc != nil {
		if err := s.subSvc.ValidateDeliveryTargetQuota(ctx, input.TenantId, input.OrgId, input.Type, s.repo); err != nil {
			if errors.Is(err, subscriptionsvc.ErrDeliveryQuotaExceeded) {
				return nil, ErrPlanLimitExceeded
			}
			return nil, err
		}
	}

	exists, err := s.repo.ExistsByNameInOrg(ctx, input.TenantId, input.OrgId, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrConflict
	}

	target := &authzmod.DeliveryTarget{
		TenantId:  input.TenantId,
		OrgId:     input.OrgId,
		Name:      input.Name,
		Type:      input.Type,
		Mode:      input.Mode,
		Enabled:   input.Enabled,
		Config:    input.Config,
		CreatedBy: input.UserId,
	}

	if err := s.repo.Insert(ctx, target); err != nil {
		return nil, err
	}

	// Permify: target#parentOrg@organization:orgId
	tuples := []map[string]interface{}{
		{
			"entity":   map[string]string{"type": "target", "id": target.TargetId},
			"relation": "parentOrg",
			"subject":  map[string]string{"type": "organization", "id": input.OrgId},
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
	if input.TenantId == "" || input.OrgId == "" {
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

	return s.repo.List(ctx, input.TenantId, input.OrgId, input.Search, page, perPage, input.SortField, input.SortOrder)
}

// ============================================================
// GetOne
// ============================================================

func (s *TargetService) GetOne(ctx context.Context, tenantId, orgId, userId, targetId string) (*authzmod.DeliveryTarget, error) {
	if tenantId == "" || orgId == "" || targetId == "" {
		return nil, ErrBadRequest
	}

	if err := s.checkAdmin(ctx, tenantId, orgId, userId); err != nil {
		return nil, err
	}

	target, err := s.repo.FindByIDAndOrg(ctx, targetId, tenantId, orgId)
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

	if input.TenantId == "" || input.OrgId == "" || input.TargetId == "" {
		return nil, ErrBadRequest
	}

	if err := s.checkAdmin(ctx, input.TenantId, input.OrgId, input.UserId); err != nil {
		return nil, err
	}

	if _, err := s.repo.FindByIDAndOrg(ctx, input.TargetId, input.TenantId, input.OrgId); err != nil {
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
		exists, err := s.repo.ExistsByNameInOrg(ctx, input.TenantId, input.OrgId, name, input.TargetId)
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
		fields["config"] = *input.Config
	}

	if len(fields) == 0 {
		return nil, ErrBadRequest
	}

	if err := s.repo.Update(ctx, input.TargetId, input.TenantId, input.OrgId, fields); err != nil {
		return nil, err
	}

	log.Info().Str("targetId", input.TargetId).Msg("delivery target updated")
	return s.repo.FindByIDAndOrg(ctx, input.TargetId, input.TenantId, input.OrgId)
}

// ============================================================
// Delete
// ============================================================

func (s *TargetService) Delete(ctx context.Context, tenantId, orgId, userId, targetId string) error {
	log := logger.FromCtx(ctx, "targetsvc", "Target.Delete")

	if tenantId == "" || orgId == "" || targetId == "" {
		return ErrBadRequest
	}

	if err := s.checkAdmin(ctx, tenantId, orgId, userId); err != nil {
		return err
	}

	if _, err := s.repo.FindByIDAndOrg(ctx, targetId, tenantId, orgId); err != nil {
		if errors.Is(err, targetrepo.ErrTargetNotFound) {
			return ErrNotFound
		}
		return err
	}

	_ = s.authzClient.DeleteEntityRelationships(ctx, tenantId, "target", targetId)

	if err := s.repo.Delete(ctx, targetId, tenantId, orgId); err != nil {
		return err
	}

	log.Info().Str("targetId", targetId).Str("orgId", orgId).Msg("delivery target deleted")
	return nil
}

// ============================================================
// ToggleEnabled
// ============================================================

func (s *TargetService) ToggleEnabled(ctx context.Context, tenantId, orgId, userId, targetId string, enabled bool) error {
	if err := s.checkAdmin(ctx, tenantId, orgId, userId); err != nil {
		return err
	}
	return s.repo.Update(ctx, targetId, tenantId, orgId, bson.M{"enabled": enabled, "updatedAt": time.Now().UTC()})
}
