// internal/services/authzsvc/memberAccessSvc.go
package authzsvc

import (
	"context"
	"sort"

	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/devmod"
)

// MemberAccessService resolves what menus and resource groups a user can access
// based on their OrgUnit membership and active permission profiles stored in MongoDB.
type MemberAccessService struct {
	permProfileRepo *authzrepo.PermissionProfileRepo
	menuProfileRepo *authzrepo.MenuPermissionProfileRepo
	authzClient     authzgw.Client
	rgRepo          *devicerepo.ResourceGroupRepo
	camRepo         *devicerepo.CameraRepo
}

func NewMemberAccessService(
	permProfileRepo *authzrepo.PermissionProfileRepo,
	menuProfileRepo *authzrepo.MenuPermissionProfileRepo,
	authzClient authzgw.Client,
	rgRepo *devicerepo.ResourceGroupRepo,
	camRepo *devicerepo.CameraRepo,
) *MemberAccessService {
	return &MemberAccessService{
		permProfileRepo: permProfileRepo,
		menuProfileRepo: menuProfileRepo,
		authzClient:     authzClient,
		rgRepo:          rgRepo,
		camRepo:         camRepo,
	}
}

// ─── Singleton for use by introspect (no DI context available there) ─────────

var defaultMemberAccessSvc *MemberAccessService

func SetDefaultMemberAccessService(svc *MemberAccessService) {
	defaultMemberAccessSvc = svc
}

// GetMenuIDsByUserOrgUnits is a package-level helper for introspect.go.
// It resolves OrgUnit memberships from Permify then looks up active profile menuIds.
func GetMenuIDsByUserOrgUnits(ctx context.Context, userID string) []string {
	if defaultMemberAccessSvc == nil {
		return nil
	}
	ids, _ := defaultMemberAccessSvc.menuIDsByUser(ctx, userID)
	return ids
}

// ─── OrgUnit membership resolution ───────────────────────────────────────────

// GetUserOrgUnitIDs fetches the orgUnit IDs the user belongs to via Permify relationships.
func (s *MemberAccessService) GetUserOrgUnitIDs(ctx context.Context, userID string) ([]string, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MemberAccess.GetUserOrgUnitIDs")
	log.Debug().Str("userId", userID).Msg("fetching orgUnit memberships from permify")

	req := &authzmod.RelationshipReadRequest{
		Filter: authzmod.RelationshipReadFilter{
			Subject: &authzmod.RelationshipEntityFilter{
				Type: "user",
				IDs:  []string{userID},
			},
			Entity: &authzmod.RelationshipEntityFilter{
				Type: "orgUnit",
			},
		},
		PageSize: 200,
		Metadata: &authzmod.RelationshipReadMetadata{},
	}

	resp, err := GetRelationships(ctx, req)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch orgUnit relationships")
		return nil, err
	}

	seen := make(map[string]bool)
	ouIDs := make([]string, 0)
	for _, t := range resp.Details {
		if t.Entity.Type == "orgUnit" && !seen[t.Entity.ID] {
			seen[t.Entity.ID] = true
			ouIDs = append(ouIDs, t.Entity.ID)
		}
	}
	log.Debug().Int("ouCount", len(ouIDs)).Msg("orgUnit memberships resolved")
	return ouIDs, nil
}

// ─── Menu access ─────────────────────────────────────────────────────────────

// MenuAccessItem represents a menu and the union of relations the caller holds on it.
type MenuAccessItem struct {
	MenuID    string   `json:"menuId"`
	Relations []string `json:"relations"`
}

// GetMemberMenuAccess returns distinct menus the user can access via active profiles,
// enriched with the union of relations across all applicable profiles.
func (s *MemberAccessService) GetMemberMenuAccess(ctx context.Context, tenantId, orgId, userID string) ([]MenuAccessItem, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MemberAccess.GetMemberMenuAccess")
	log.Debug().Str("userId", userID).Str("orgId", orgId).Msg("resolving member menu access")

	ouIDs, err := s.GetUserOrgUnitIDs(ctx, userID)
	if err != nil || len(ouIDs) == 0 {
		return []MenuAccessItem{}, nil
	}

	profiles, err := s.menuProfileRepo.FindActiveByOrgUnitIDs(ctx, tenantId, orgId, ouIDs)
	if err != nil {
		return nil, err
	}

	// menuId → union of relations
	relMap := make(map[string]map[string]bool)
	for _, p := range profiles {
		rels := p.Relations
		if len(rels) == 0 {
			rels = []string{"viewer"}
		}
		for _, menuID := range p.MenuIDs {
			if relMap[menuID] == nil {
				relMap[menuID] = make(map[string]bool)
			}
			for _, rel := range rels {
				relMap[menuID][rel] = true
			}
		}
	}

	result := make([]MenuAccessItem, 0, len(relMap))
	for menuID, relSet := range relMap {
		rels := make([]string, 0, len(relSet))
		for rel := range relSet {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		result = append(result, MenuAccessItem{MenuID: menuID, Relations: rels})
	}
	log.Debug().Int("menuCount", len(result)).Msg("member menu access resolved")
	return result, nil
}

// menuIDsByUser is the internal variant used by the introspect singleton (no orgId filter).
func (s *MemberAccessService) menuIDsByUser(ctx context.Context, userID string) ([]string, error) {
	ouIDs, err := s.GetUserOrgUnitIDs(ctx, userID)
	if err != nil || len(ouIDs) == 0 {
		return []string{}, nil
	}
	return s.menuProfileRepo.FindActiveMenuIDsByOrgUnitIDs(ctx, ouIDs)
}

// ─── Resource access — group list (legacy / internal) ────────────────────────

// ResourceAccessItem represents a resource group and the relations the user has on it.
type ResourceAccessItem struct {
	ResourceGroupID string   `json:"resourceGroupId"`
	Relations       []string `json:"relations"`
}

// buildRgRelMap resolves the active profiles for the user and returns a map of
// resourceGroupId → union of relations across all applicable profiles.
func (s *MemberAccessService) buildRgRelMap(ctx context.Context, tenantId, orgId, userID string) (map[string]map[string]bool, error) {
	ouIDs, err := s.GetUserOrgUnitIDs(ctx, userID)
	if err != nil || len(ouIDs) == 0 {
		return map[string]map[string]bool{}, nil
	}

	profiles, err := s.permProfileRepo.FindActiveByOrgUnitIDs(ctx, tenantId, orgId, ouIDs)
	if err != nil {
		return nil, err
	}

	relMap := make(map[string]map[string]bool)
	for _, p := range profiles {
		for _, rgID := range p.ResourceGroupIDs {
			if relMap[rgID] == nil {
				relMap[rgID] = make(map[string]bool)
			}
			for _, rel := range p.Relations {
				relMap[rgID][rel] = true
			}
		}
	}
	return relMap, nil
}

// ─── Resource access — paginated camera list ─────────────────────────────────

// CameraWithRelations is a camera DTO enriched with the union of Permify relations
// the caller has on it (via their resource group profiles).
type CameraWithRelations struct {
	devmod.CameraDTO
	Relations []string `json:"relations"`
}

// MemberCameraAccessParams carries pagination + filter options for GetMemberCameraAccess.
type MemberCameraAccessParams struct {
	ResourceType string
	Search       string
	Page         int
	PerPage      int
	SortField    string
	SortOrder    string
}

// MemberCameraAccessResult is the paginated response for GetMemberCameraAccess.
type MemberCameraAccessResult struct {
	Cameras      []CameraWithRelations `json:"cameras"`
	TotalRecords int64                 `json:"totalRecords"`
	TotalPages   int64                 `json:"totalPages"`
	Page         int                   `json:"page"`
	PerPage      int                   `json:"perPage"`
}

// GetMemberCameraAccess returns a paginated list of cameras the user can access,
// enriched with the union of Permify relations (viewer, editor, …) they hold.
//
// Flow:
//  1. Resolve user's OrgUnit IDs → active PermissionProfiles → rgId→relations map.
//  2. (Optional) filter rgIds by resourceType via rgRepo.FindByGroupIDs.
//  3. For each valid rgId, call ListRelationshipsBySubject to get camIds; build camId→relations map.
//  4. Paginate + search camIds via camRepo.FindByCamIDs.
//  5. Attach relations to each CameraDTO.
func (s *MemberAccessService) GetMemberCameraAccess(
	ctx context.Context,
	tenantId, orgId, userID string,
	params MemberCameraAccessParams,
) (*MemberCameraAccessResult, error) {
	log := logger.FromCtx(ctx, "authzsvc", "MemberAccess.GetMemberCameraAccess")
	log.Debug().
		Str("userId", userID).
		Str("orgId", orgId).
		Str("resourceType", params.ResourceType).
		Msg("resolving member camera access")

	if params.Page < 1 {
		params.Page = 1
	}
	if params.PerPage <= 0 {
		params.PerPage = 10
	}

	// 1. Build rgId → relations map from active profiles
	relMap, err := s.buildRgRelMap(ctx, tenantId, orgId, userID)
	if err != nil {
		return nil, err
	}
	if len(relMap) == 0 {
		return &MemberCameraAccessResult{
			Cameras: []CameraWithRelations{},
			Page:    params.Page,
			PerPage: params.PerPage,
		}, nil
	}

	// 2. Collect all rgIds from the profile map
	allRgIDs := make([]string, 0, len(relMap))
	for rgID := range relMap {
		allRgIDs = append(allRgIDs, rgID)
	}

	// 3. (Optional) filter by resourceType — keep only rgIds whose group matches
	validRgIDs := allRgIDs
	if params.ResourceType != "" && s.rgRepo != nil {
		log.Debug().Str("resourceType", params.ResourceType).Msg("filtering resource groups by type")
		groups, _, err := s.rgRepo.FindByGroupIDs(ctx, tenantId, orgId, allRgIDs, devicerepo.ListOptions{
			ResourceType: params.ResourceType,
			PerPages:     len(allRgIDs) + 1,
		})
		if err != nil {
			return nil, err
		}
		validRgIDs = make([]string, 0, len(groups))
		for _, g := range groups {
			validRgIDs = append(validRgIDs, g.GroupID)
		}
		log.Debug().Int("validGroupCount", len(validRgIDs)).Msg("resource groups after type filter")
	}

	if len(validRgIDs) == 0 {
		return &MemberCameraAccessResult{
			Cameras: []CameraWithRelations{},
			Page:    params.Page,
			PerPage: params.PerPage,
		}, nil
	}

	// 4. Fetch camera IDs from Permify for each resource group
	//    tuple pattern: (resource:camId, parentGroup, resourceGroup:rgId)
	camRelMap := make(map[string]map[string]bool) // camId → set of relations
	for _, rgID := range validRgIDs {
		tuples, err := s.authzClient.ListRelationshipsBySubject(ctx, tenantId, "resourceGroup", rgID)
		if err != nil {
			log.Warn().Err(err).Str("rgId", rgID).Msg("failed to list camera tuples for group")
			continue
		}
		rgRels := relMap[rgID]
		for _, t := range tuples {
			if t.Entity.Type != "resource" || t.Relation != "parentGroup" {
				continue
			}
			camID := t.Entity.ID
			if camRelMap[camID] == nil {
				camRelMap[camID] = make(map[string]bool)
			}
			for rel := range rgRels {
				camRelMap[camID][rel] = true
			}
		}
	}
	log.Debug().Int("camCount", len(camRelMap)).Msg("cameras resolved from permify")

	if len(camRelMap) == 0 {
		return &MemberCameraAccessResult{
			Cameras: []CameraWithRelations{},
			Page:    params.Page,
			PerPage: params.PerPage,
		}, nil
	}

	// 5. Fetch camera documents with pagination/search
	allCamIDs := make([]string, 0, len(camRelMap))
	for camID := range camRelMap {
		allCamIDs = append(allCamIDs, camID)
	}

	docs, total, err := s.camRepo.FindByCamIDs(
		ctx,
		allCamIDs,
		params.Search,
		params.SortField,
		params.SortOrder,
		params.Page,
		params.PerPage,
	)
	if err != nil {
		return nil, err
	}

	// 6. Enrich DTOs with sorted relations
	cameras := make([]CameraWithRelations, 0, len(docs))
	for i := range docs {
		dto := devicerepo.ToDTO(&docs[i])
		rels := make([]string, 0, len(camRelMap[dto.ID]))
		for rel := range camRelMap[dto.ID] {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		cameras = append(cameras, CameraWithRelations{
			CameraDTO: dto,
			Relations: rels,
		})
	}

	totalPages := (total + int64(params.PerPage) - 1) / int64(params.PerPage)
	if totalPages == 0 {
		totalPages = 1
	}

	log.Debug().
		Int64("total", total).
		Int("page", params.Page).
		Msg("member camera access resolved")

	return &MemberCameraAccessResult{
		Cameras:      cameras,
		TotalRecords: total,
		TotalPages:   totalPages,
		Page:         params.Page,
		PerPage:      params.PerPage,
	}, nil
}
