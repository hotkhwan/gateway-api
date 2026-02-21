// internal/services/authzsvc/orgUnit.go
package authzsvc

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type OrgUnitService struct {
	orgUnitRepo *authzrepo.OrgUnitRepo
	authzClient authzgw.Client
	idClient    *authgw.Client
}

type OrgUnitNode struct {
	Id          string        `json:"id"`
	ParentId    *string       `json:"parentId,omitempty"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	IsRoot      bool          `json:"isRoot"`
	Children    []OrgUnitNode `json:"children"`
	Orphaned    bool          `json:"orphaned,omitempty"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

func NewOrgUnitService(
	orgUnitRepo *authzrepo.OrgUnitRepo,
	authzClient authzgw.Client,
	idClient *authgw.Client,
) *OrgUnitService {

	if orgUnitRepo == nil {
		panic("orgUnitRepo required")
	}
	if authzClient == nil {
		panic("authzClient required")
	}

	return &OrgUnitService{
		orgUnitRepo: orgUnitRepo,
		authzClient: authzClient,
		idClient:    idClient,
	}
}

func buildOrgUnitTree(units []authzmod.OrgUnit) []OrgUnitNode {

	nodeMap := make(map[string]*OrgUnitNode)
	var roots []*OrgUnitNode

	// create nodes
	for _, u := range units {
		nodeMap[u.UnitId] = &OrgUnitNode{
			Id:          u.UnitId,
			ParentId:    u.ParentUnitId,
			Name:        u.Name,
			Description: u.Description,
			IsRoot:      u.IsRoot,
			Children:    []OrgUnitNode{},
			CreatedAt:   u.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   u.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
	}

	// attach children
	for _, node := range nodeMap {

		if node.ParentId == nil {
			roots = append(roots, node)
			continue
		}

		parent, ok := nodeMap[*node.ParentId]
		if !ok {
			node.Orphaned = true
			roots = append(roots, node)
			continue
		}

		parent.Children = append(parent.Children, *node)
	}

	return derefNodes(roots)
}

func derefNodes(nodes []*OrgUnitNode) []OrgUnitNode {
	result := make([]OrgUnitNode, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, *n)
	}
	return result
}

func (s *OrgUnitService) GetOrgUnitTree(
	ctx context.Context,
	tenantId string,
	orgId string,
) ([]OrgUnitNode, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"orgUnit.tree",
		"authzsvc",
		"GetOrgUnitTree",
	)
	defer end()

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)

	if tenantId == "" || orgId == "" {
		return nil, ErrUnauthorized
	}

	units, err := s.orgUnitRepo.ListByOrg(ctx, tenantId, orgId)
	if err != nil {
		return nil, err
	}

	tree := buildOrgUnitTree(units)

	log.Info().
		Int("unitCount", len(units)).
		Int("rootCount", len(tree)).
		Msg("orgUnit tree built")

	return tree, nil
}

func (s *OrgUnitService) CreateOrgUnit(
	ctx context.Context,
	tenantId string,
	orgId string,
	name string,
	description string,
	parentId *string,
	createdBy string,
) (string, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"orgUnit.create",
		"authzsvc",
		"CreateOrgUnit",
	)
	defer end()

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)

	unitId := uuid.NewString()
	now := time.Now().UTC()

	unit := &authzmod.OrgUnit{
		ID:           primitive.NewObjectID(),
		UnitId:       unitId,
		TenantId:     tenantId,
		OrgId:        orgId,
		ParentUnitId: parentId,
		Name:         name,
		Description:  description,
		IsRoot:       parentId == nil,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedBy:    createdBy,
		UpdatedAt:    now,
	}

	if err := s.orgUnitRepo.Insert(ctx, unit); err != nil {
		return "", err
	}

	tuples := []map[string]interface{}{
		{
			"entity":   map[string]interface{}{"type": "orgUnit", "id": unitId},
			"relation": "parentOrg",
			"subject":  map[string]interface{}{"type": "organization", "id": orgId},
		},
	}
	if parentId != nil {

		parent, err := s.orgUnitRepo.FindByUnitId(
			ctx,
			tenantId,
			orgId,
			*parentId,
		)

		if err != nil {
			return "", err
		}

		if parent == nil {
			return "", ErrInvalidParent
		}
	}

	if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
		_ = s.orgUnitRepo.DeleteByUnitId(ctx, tenantId, orgId, unitId)
		return "", err
	}

	log.Info().Str("unitId", unitId).Msg("orgUnit created")

	return unitId, nil
}

func (s *OrgUnitService) UpdateOrgUnit(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
	name string,
	description string,
	updatedBy string,
) error {

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	unitId = strings.TrimSpace(unitId)

	unit, err := s.orgUnitRepo.FindByUnitId(ctx, tenantId, orgId, unitId)
	if err != nil {
		return err
	}
	if unit == nil {
		return ErrNotFound
	}

	return s.orgUnitRepo.UpdateMetadata(
		ctx,
		tenantId,
		orgId,
		unitId,
		name,
		description,
		updatedBy,
	)
}

func (s *OrgUnitService) DeleteOrgUnit(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
) error {

	unit, err := s.orgUnitRepo.FindByUnitId(ctx, tenantId, orgId, unitId)
	if err != nil {
		return err
	}
	if unit == nil {
		return nil
	}

	children, err := s.orgUnitRepo.FindChildren(ctx, tenantId, orgId, unitId)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return ErrConflict
	}

	if err := s.authzClient.DeleteEntityRelationships(
		ctx,
		tenantId,
		"orgUnit",
		unitId,
	); err != nil {
		return err
	}

	return s.orgUnitRepo.DeleteByUnitId(ctx, tenantId, orgId, unitId)
}
