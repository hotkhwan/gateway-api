// internal/services/authzsvc/permissionProfileSvc.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

type PermissionProfileService struct {
	profileRepo *authzrepo.PermissionProfileRepo
	ouRepo      *authzrepo.OrgUnitRepo
	rgRepo      *devicerepo.ResourceGroupRepo
	authzClient authzgw.Client
}

func NewPermissionProfileService(
	profileRepo *authzrepo.PermissionProfileRepo,
	ouRepo *authzrepo.OrgUnitRepo,
	rgRepo *devicerepo.ResourceGroupRepo,
	authzClient authzgw.Client,
) *PermissionProfileService {
	return &PermissionProfileService{
		profileRepo: profileRepo,
		ouRepo:      ouRepo,
		rgRepo:      rgRepo,
		authzClient: authzClient,
	}
}

// ============================================================
// Input types
// ============================================================

type CreatePermProfileInput struct {
	TenantID    string
	OrgID       string
	CallerID    string
	Name        string
	Description string
	Status      bool
	Relations   []string
}

type UpdatePermProfileInput struct {
	TenantID         string
	OrgID            string
	ProfileID        string
	CallerID         string
	Name             string
	Description      string
	Status           bool
	Relations        []string
	OrgUnitIDs       []string
	ResourceGroupIDs []string
}

type ListPermProfileInput struct {
	TenantID  string
	OrgID     string
	Search    string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}

// ============================================================
// validProfileRelations
// ============================================================

var validProfileRelations = map[string]bool{
	"creator": true,
	"viewer":  true,
	"editor":  true,
	"deleter": true,
}

func validateRelations(relations []string) error {
	for _, r := range relations {
		if !validProfileRelations[strings.ToLower(r)] {
			return fmt.Errorf("%w: invalid relation '%s', must be creator|viewer|editor|deleter", ErrInvalidArgs, r)
		}
	}
	return nil
}

// ============================================================
// Create
// ============================================================

func (s *PermissionProfileService) Create(ctx context.Context, input CreatePermProfileInput) (*authzmod.PermissionProfile, error) {
	log := logger.FromCtx(ctx, "authzsvc", "PermissionProfile.Create")

	if input.TenantID == "" || input.OrgID == "" || input.Name == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}
	if err := validateRelations(input.Relations); err != nil {
		return nil, err
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, ErrForbidden
	}

	exists, err := s.profileRepo.ExistsByNameInOrg(ctx, input.TenantID, input.OrgID, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, authzrepo.ErrPermProfileNameExists
	}

	p := &authzmod.PermissionProfile{
		TenantID:         input.TenantID,
		OrgID:            input.OrgID,
		Name:             input.Name,
		Description:      input.Description,
		Status:           input.Status,
		Relations:        input.Relations,
		OrgUnitIDs:       []string{},
		ResourceGroupIDs: []string{},
		CreatedBy:        input.CallerID,
	}
	if err := s.profileRepo.Insert(ctx, p); err != nil {
		return nil, err
	}

	log.Info().Str("profileId", p.ProfileID).Msg("✅ PermissionProfile created")
	return p, nil
}

// ============================================================
// Update
// Tuple reconciliation: delete old → write new
// ============================================================

func (s *PermissionProfileService) Update(ctx context.Context, input UpdatePermProfileInput) (*authzmod.PermissionProfile, error) {
	log := logger.FromCtx(ctx, "authzsvc", "PermissionProfile.Update")

	if input.TenantID == "" || input.OrgID == "" || input.ProfileID == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}
	if err := validateRelations(input.Relations); err != nil {
		return nil, err
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, ErrForbidden
	}

	// Load old profile
	old, err := s.profileRepo.FindByIDAndOrg(ctx, input.ProfileID, input.TenantID, input.OrgID)
	if err != nil {
		return nil, err
	}

	// Validate name uniqueness (if name changed)
	if input.Name != "" && input.Name != old.Name {
		exists, err := s.profileRepo.ExistsByNameInOrg(ctx, input.TenantID, input.OrgID, input.Name, input.ProfileID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, authzrepo.ErrPermProfileNameExists
		}
	}

	// ── Resolve new OUs (nil = not provided → keep old) ─────────
	// nil  = field not sent in JSON  → keep old value
	// []   = explicitly cleared      → store empty
	// ["x"] = update to these IDs   → validate each
	var validOUIDs []string
	if input.OrgUnitIDs == nil {
		validOUIDs = old.OrgUnitIDs
	} else {
		validOUIDs = make([]string, 0, len(input.OrgUnitIDs))
		for _, ouID := range input.OrgUnitIDs {
			exists, err := s.ouRepo.ExistsByIDAndOrg(ctx, input.TenantID, input.OrgID, ouID)
			if err != nil || !exists {
				return nil, fmt.Errorf("%w: orgUnit not found: %s", ErrNotFound, ouID)
			}
			validOUIDs = append(validOUIDs, ouID)
		}
	}

	// ── Resolve new RGs (nil = not provided → keep old) ──────────
	var validRGIDs []string
	if input.ResourceGroupIDs == nil {
		validRGIDs = old.ResourceGroupIDs
	} else {
		validRGIDs = make([]string, 0, len(input.ResourceGroupIDs))
		for _, rgID := range input.ResourceGroupIDs {
			if _, err := s.rgRepo.FindByIDAndOrg(ctx, rgID, input.TenantID, input.OrgID); err != nil {
				return nil, fmt.Errorf("%w: resourceGroup not found: %s", ErrNotFound, rgID)
			}
			validRGIDs = append(validRGIDs, rgID)
		}
	}

	// ── Delete old tuples (before applying new state) ────────────
	if old.Status && len(old.OrgUnitIDs) > 0 && len(old.ResourceGroupIDs) > 0 {
		if err := s.deleteTuples(ctx, input.TenantID, old.Relations, old.OrgUnitIDs, old.ResourceGroupIDs); err != nil {
			log.Warn().Err(err).Msg("⚠️ partial tuple delete on update")
		}
	}

	// ── Resolve other fields (nil/zero = keep old) ───────────────
	newName := input.Name
	if newName == "" {
		newName = old.Name
	}
	newDesc := input.Description
	if newDesc == "" {
		newDesc = old.Description
	}
	newRelations := input.Relations
	if len(newRelations) == 0 {
		newRelations = old.Relations
	}

	if err := s.profileRepo.Update(ctx, input.ProfileID, input.TenantID, input.OrgID, bson.M{
		"name":             newName,
		"description":      newDesc,
		"status":           input.Status,
		"relations":        newRelations,
		"orgUnitIds":       validOUIDs,
		"resourceGroupIds": validRGIDs,
	}); err != nil {
		return nil, err
	}

	// ── Write new tuples (if new state is active) ───────────────
	if input.Status && len(validOUIDs) > 0 && len(validRGIDs) > 0 {
		if err := s.writeTuples(ctx, input.TenantID, newRelations, validOUIDs, validRGIDs); err != nil {
			return nil, fmt.Errorf("permify sync failed: %w", err)
		}
	}

	updated, err := s.profileRepo.FindByIDAndOrg(ctx, input.ProfileID, input.TenantID, input.OrgID)
	if err != nil {
		return nil, err
	}

	log.Info().Str("profileId", input.ProfileID).Msg("✅ PermissionProfile updated")
	return updated, nil
}

// ============================================================
// Delete
// ============================================================

func (s *PermissionProfileService) Delete(ctx context.Context, tenantId, orgId, profileId, callerID string) error {
	log := logger.FromCtx(ctx, "authzsvc", "PermissionProfile.Delete")

	if err := s.guardManageOrg(ctx, tenantId, orgId, callerID); err != nil {
		return ErrForbidden
	}

	p, err := s.profileRepo.FindByIDAndOrg(ctx, profileId, tenantId, orgId)
	if err != nil {
		return err
	}

	// Delete tuples if profile was active
	if p.Status && len(p.OrgUnitIDs) > 0 && len(p.ResourceGroupIDs) > 0 {
		if err := s.deleteTuples(ctx, tenantId, p.Relations, p.OrgUnitIDs, p.ResourceGroupIDs); err != nil {
			log.Warn().Err(err).Msg("⚠️ partial tuple delete on profile delete")
		}
	}

	if err := s.profileRepo.Delete(ctx, profileId, tenantId, orgId); err != nil {
		return err
	}

	log.Info().Str("profileId", profileId).Msg("✅ PermissionProfile deleted")
	return nil
}

// ============================================================
// Get
// ============================================================

func (s *PermissionProfileService) Get(ctx context.Context, tenantId, orgId, profileId string) (*authzmod.PermissionProfile, error) {
	return s.profileRepo.FindByIDAndOrg(ctx, profileId, tenantId, orgId)
}

// ============================================================
// List
// ============================================================

func (s *PermissionProfileService) List(ctx context.Context, input ListPermProfileInput) ([]authzmod.PermissionProfile, int64, error) {
	return s.profileRepo.List(
		ctx,
		input.TenantID, input.OrgID, input.Search,
		input.Page, input.PerPage,
		input.SortField, input.SortOrder,
	)
}

// ============================================================
// Internal helpers
// ============================================================

func (s *PermissionProfileService) guardManageOrg(ctx context.Context, tenantId, orgId, callerUserId string) error {
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, "", "organization", orgId, "manage", "user", callerUserId,
	)
	if err != nil {
		return fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// writeTuples: Cartesian product relations × OUs × RGs → Permify
func (s *PermissionProfileService) writeTuples(ctx context.Context, tenantId string, relations, ouIDs, rgIDs []string) error {
	tuples := make([]map[string]any, 0, len(relations)*len(ouIDs)*len(rgIDs))
	for _, rgID := range rgIDs {
		for _, ouID := range ouIDs {
			for _, rel := range relations {
				tuples = append(tuples, map[string]any{
					"entity":   map[string]any{"type": "resourceGroup", "id": rgID},
					"relation": rel,
					"subject":  map[string]any{"type": "orgUnit", "id": ouID},
				})
			}
		}
	}
	if len(tuples) == 0 {
		return nil
	}
	return s.authzClient.WriteTuples(ctx, tenantId, tuples)
}

// deleteTuples: delete all combinations relations × OUs × RGs from Permify
func (s *PermissionProfileService) deleteTuples(ctx context.Context, tenantId string, relations, ouIDs, rgIDs []string) error {
	var lastErr error
	for _, rgID := range rgIDs {
		for _, ouID := range ouIDs {
			for _, rel := range relations {
				if err := s.authzClient.DeleteSpecificTupleWithRelation(
					ctx, tenantId,
					"resourceGroup", rgID, rel,
					"orgUnit", ouID,
				); err != nil {
					lastErr = err
				}
			}
		}
	}
	return lastErr
}
