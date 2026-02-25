// internal/services/authzsvc/menuPermissionProfileSvc.go
package authzsvc

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

// MenuPermissionProfileService manages menu permission profiles.
// Menu entity only supports "viewer" relation for orgUnit in the Permify schema.
type ProfileMenuPermissionService struct {
	profileRepo *authzrepo.MenuPermissionProfileRepo
	ouRepo      *authzrepo.OrgUnitRepo
	menuRepo    *authzrepo.MenuListRepo
	authzClient authzgw.Client
}

func NewMenuPermissionProfileService(
	profileRepo *authzrepo.MenuPermissionProfileRepo,
	ouRepo *authzrepo.OrgUnitRepo,
	menuRepo *authzrepo.MenuListRepo,
	authzClient authzgw.Client,
) *ProfileMenuPermissionService {
	return &ProfileMenuPermissionService{
		profileRepo: profileRepo,
		ouRepo:      ouRepo,
		menuRepo:    menuRepo,
		authzClient: authzClient,
	}
}

// ============================================================
// Input types
// ============================================================

type CreateMenuProfileInput struct {
	TenantID    string
	OrgID       string
	CallerID    string
	Name        string
	Description string
	Status      bool
	MenuIDs     []string
	// Relations ที่ orgUnit จะได้รับบน menu entity เช่น ["viewer","creator"]
	// ถ้าว่างเปล่า ใช้ค่า default ["viewer"]
	Relations []string
}

type UpdateMenuProfileInput struct {
	TenantID    string
	OrgID       string
	ProfileID   string
	CallerID    string
	Name        string
	Description string
	Status      bool
	// nil = not provided (keep old), [] = clear/reset, ["id"] = set
	OrgUnitIDs []string
	MenuIDs    []string
	Relations  []string // nil = keep old, [] = reset to ["viewer"], [...] = set
}

type ListMenuProfileInput struct {
	TenantID  string
	OrgID     string
	Search    string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}

// ============================================================
// Create
// ============================================================

func (s *ProfileMenuPermissionService) Create(ctx context.Context, input CreateMenuProfileInput) (*authzmod.MenuPermissionProfile, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.Create")
	log.Debug().Str("orgId", input.OrgID).Str("name", input.Name).Int("menuCount", len(input.MenuIDs)).Msg("creating menu permission profile")

	if input.TenantID == "" || input.OrgID == "" || input.Name == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, ErrForbidden
	}
	log.Debug().Msg("permission guard passed")

	exists, err := s.profileRepo.ExistsByNameInOrg(ctx, input.TenantID, input.OrgID, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, authzrepo.ErrMenuProfileNameExists
	}
	log.Debug().Str("name", input.Name).Msg("name uniqueness check passed")

	// Validate menuIds against the known list
	validMenuIDs, err := s.validateMenuIDs(ctx, input.MenuIDs)
	if err != nil {
		return nil, err
	}
	log.Debug().Int("validMenuCount", len(validMenuIDs)).Msg("menuIds validated")

	relations := input.Relations
	if len(relations) == 0 {
		relations = []string{"viewer"}
	}

	p := &authzmod.MenuPermissionProfile{
		TenantID:    input.TenantID,
		OrgID:       input.OrgID,
		Name:        input.Name,
		Description: input.Description,
		Status:      input.Status,
		OrgUnitIDs:  []string{},
		MenuIDs:     validMenuIDs,
		Relations:   relations,
		CreatedBy:   input.CallerID,
	}
	if err := s.profileRepo.Insert(ctx, p); err != nil {
		return nil, err
	}

	log.Info().Str("profileId", p.ProfileID).Msg("✅ MenuPermissionProfile created")
	return p, nil
}

// ============================================================
// Update
// Tuple reconciliation: delete old → write new
// ============================================================

func (s *ProfileMenuPermissionService) Update(ctx context.Context, input UpdateMenuProfileInput) (*authzmod.MenuPermissionProfile, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.Update")
	log.Debug().Str("profileId", input.ProfileID).Str("orgId", input.OrgID).Msg("updating menu permission profile")

	if input.TenantID == "" || input.OrgID == "" || input.ProfileID == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, ErrForbidden
	}
	log.Debug().Msg("permission guard passed")

	old, err := s.profileRepo.FindByIDAndOrg(ctx, input.ProfileID, input.TenantID, input.OrgID)
	if err != nil {
		return nil, err
	}

	// Validate name uniqueness if changed
	if input.Name != "" && input.Name != old.Name {
		exists, err := s.profileRepo.ExistsByNameInOrg(ctx, input.TenantID, input.OrgID, input.Name, input.ProfileID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, authzrepo.ErrMenuProfileNameExists
		}
		log.Debug().Str("newName", input.Name).Msg("name uniqueness check passed")
	}

	// ── Resolve new OUs (nil = not provided → keep old) ──────────
	var validOUIDs []string
	if input.OrgUnitIDs == nil {
		validOUIDs = old.OrgUnitIDs
		log.Debug().Int("ouCount", len(validOUIDs)).Msg("orgUnitIds not provided, keeping old")
	} else {
		validOUIDs = make([]string, 0, len(input.OrgUnitIDs))
		for _, ouID := range input.OrgUnitIDs {
			exists, err := s.ouRepo.ExistsByIDAndOrg(ctx, input.TenantID, input.OrgID, ouID)
			if err != nil || !exists {
				return nil, fmt.Errorf("%w: orgUnit not found: %s", ErrNotFound, ouID)
			}
			validOUIDs = append(validOUIDs, ouID)
		}
		log.Debug().Int("ouCount", len(validOUIDs)).Msg("orgUnitIds resolved")
	}

	// ── Resolve new menuIds (nil = not provided → keep old) ───────
	var validMenuIDs []string
	if input.MenuIDs == nil {
		validMenuIDs = old.MenuIDs
		log.Debug().Int("menuCount", len(validMenuIDs)).Msg("menuIds not provided, keeping old")
	} else {
		validMenuIDs, err = s.validateMenuIDs(ctx, input.MenuIDs)
		if err != nil {
			return nil, err
		}
		log.Debug().Int("menuCount", len(validMenuIDs)).Msg("menuIds resolved")
	}

	// ── Resolve relations (nil = not provided → keep old) ────────
	var validRelations []string
	if input.Relations == nil {
		validRelations = old.Relations
		if len(validRelations) == 0 {
			validRelations = []string{"viewer"}
		}
		log.Debug().Int("relCount", len(validRelations)).Msg("relations not provided, keeping old")
	} else if len(input.Relations) == 0 {
		validRelations = []string{"viewer"} // reset to default
		log.Debug().Msg("relations reset to default [viewer]")
	} else {
		validRelations = input.Relations
		log.Debug().Int("relCount", len(validRelations)).Msg("relations resolved")
	}

	// ── Delete old tuples (before applying new state) ─────────────
	if old.Status && len(old.OrgUnitIDs) > 0 && len(old.MenuIDs) > 0 {
		oldRelations := old.Relations
		if len(oldRelations) == 0 {
			oldRelations = []string{"viewer"}
		}
		log.Debug().Int("ouCount", len(old.OrgUnitIDs)).Int("menuCount", len(old.MenuIDs)).Msg("deleting old permify tuples")
		if err := s.deleteMenuTuples(ctx, input.TenantID, old.OrgUnitIDs, old.MenuIDs, oldRelations); err != nil {
			log.Warn().Err(err).Msg("⚠️ partial tuple delete on menu profile update")
		}
	}

	// ── Resolve other fields ──────────────────────────────────────
	newName := input.Name
	if newName == "" {
		newName = old.Name
	}
	newDesc := input.Description
	if newDesc == "" {
		newDesc = old.Description
	}

	if err := s.profileRepo.Update(ctx, input.ProfileID, input.TenantID, input.OrgID, bson.M{
		"name":        newName,
		"description": newDesc,
		"status":      input.Status,
		"orgUnitIds":  validOUIDs,
		"menuIds":     validMenuIDs,
		"relations":   validRelations,
	}); err != nil {
		return nil, err
	}

	// ── Write new tuples (if active) ─────────────────────────────
	if input.Status && len(validOUIDs) > 0 && len(validMenuIDs) > 0 {
		log.Debug().Int("ouCount", len(validOUIDs)).Int("menuCount", len(validMenuIDs)).Msg("writing new permify tuples")
		if err := s.writeMenuTuples(ctx, input.TenantID, validOUIDs, validMenuIDs, validRelations); err != nil {
			return nil, fmt.Errorf("permify sync failed: %w", err)
		}
	}

	updated, err := s.profileRepo.FindByIDAndOrg(ctx, input.ProfileID, input.TenantID, input.OrgID)
	if err != nil {
		return nil, err
	}

	log.Info().Str("profileId", input.ProfileID).Msg("✅ MenuPermissionProfile updated")
	return updated, nil
}

// ============================================================
// Delete
// ============================================================

func (s *ProfileMenuPermissionService) Delete(ctx context.Context, tenantId, orgId, profileId, callerID string) error {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.Delete")
	log.Debug().Str("profileId", profileId).Str("orgId", orgId).Msg("deleting menu permission profile")

	if err := s.guardManageOrg(ctx, tenantId, orgId, callerID); err != nil {
		return ErrForbidden
	}
	log.Debug().Msg("permission guard passed")

	p, err := s.profileRepo.FindByIDAndOrg(ctx, profileId, tenantId, orgId)
	if err != nil {
		return err
	}

	if p.Status && len(p.OrgUnitIDs) > 0 && len(p.MenuIDs) > 0 {
		storedRels := p.Relations
		if len(storedRels) == 0 {
			storedRels = []string{"viewer"}
		}
		log.Debug().Int("ouCount", len(p.OrgUnitIDs)).Int("menuCount", len(p.MenuIDs)).Msg("revoking permify tuples before delete")
		if err := s.deleteMenuTuples(ctx, tenantId, p.OrgUnitIDs, p.MenuIDs, storedRels); err != nil {
			log.Warn().Err(err).Msg("⚠️ partial tuple delete on menu profile delete")
		}
	}

	if err := s.profileRepo.Delete(ctx, profileId, tenantId, orgId); err != nil {
		return err
	}

	log.Info().Str("profileId", profileId).Msg("✅ MenuPermissionProfile deleted")
	return nil
}

// ============================================================
// Get / List
// ============================================================

func (s *ProfileMenuPermissionService) Get(ctx context.Context, tenantId, orgId, profileId string) (*authzmod.MenuPermissionProfile, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.Get")
	log.Debug().Str("profileId", profileId).Str("orgId", orgId).Msg("fetching menu permission profile")
	return s.profileRepo.FindByIDAndOrg(ctx, profileId, tenantId, orgId)
}

func (s *ProfileMenuPermissionService) List(ctx context.Context, input ListMenuProfileInput) ([]authzmod.MenuPermissionProfile, int64, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.List")
	log.Debug().Str("orgId", input.OrgID).Str("search", input.Search).Int("page", input.Page).Int("perPage", input.PerPage).Msg("listing menu permission profiles")
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

func (s *ProfileMenuPermissionService) guardManageOrg(ctx context.Context, tenantId, orgId, callerUserId string) error {
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

// validateMenuIDs checks that all provided menuIds exist in the menu list.
func (s *ProfileMenuPermissionService) validateMenuIDs(ctx context.Context, menuIDs []string) ([]string, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.validateMenuIDs")
	if len(menuIDs) == 0 {
		return []string{}, nil
	}
	validSet, err := s.menuRepo.ValidMenuIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load menu list: %w", err)
	}
	result := make([]string, 0, len(menuIDs))
	for _, id := range menuIDs {
		if !validSet[id] {
			return nil, fmt.Errorf("%w: menu not found: %s", ErrNotFound, id)
		}
		result = append(result, id)
	}
	log.Debug().Int("requested", len(menuIDs)).Int("valid", len(result)).Msg("menuIds validation complete")
	return result, nil
}

// writeMenuTuples: OUs × menus × relations → Permify
func (s *ProfileMenuPermissionService) writeMenuTuples(ctx context.Context, tenantId string, ouIDs, menuIDs, relations []string) error {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.writeMenuTuples")
	if len(relations) == 0 {
		relations = []string{"viewer"}
	}
	tuples := make([]map[string]any, 0, len(ouIDs)*len(menuIDs)*len(relations))
	for _, rel := range relations {
		for _, menuID := range menuIDs {
			for _, ouID := range ouIDs {
				tuples = append(tuples, map[string]any{
					"entity":   map[string]any{"type": "menu", "id": menuID},
					"relation": rel,
					"subject":  map[string]any{"type": "orgUnit", "id": ouID},
				})
			}
		}
	}
	if len(tuples) == 0 {
		return nil
	}
	log.Debug().Int("tupleCount", len(tuples)).Msg("writing menu tuples to permify")
	return s.authzClient.WriteTuples(ctx, tenantId, tuples)
}

// deleteMenuTuples: delete all OUs × menus × relations combinations from Permify
func (s *ProfileMenuPermissionService) deleteMenuTuples(ctx context.Context, tenantId string, ouIDs, menuIDs, relations []string) error {
	log := logger.FromCtx(ctx, "authzsvc", "MenuPermissionProfile.deleteMenuTuples")
	if len(relations) == 0 {
		relations = []string{"viewer"}
	}
	total := len(ouIDs) * len(menuIDs) * len(relations)
	log.Debug().Int("tupleCount", total).Msg("deleting menu tuples from permify")
	var lastErr error
	for _, rel := range relations {
		for _, menuID := range menuIDs {
			for _, ouID := range ouIDs {
				if err := s.authzClient.DeleteSpecificTupleWithRelation(
					ctx, tenantId,
					"menu", menuID, rel,
					"orgUnit", ouID,
				); err != nil {
					lastErr = err
				}
			}
		}
	}
	return lastErr
}
