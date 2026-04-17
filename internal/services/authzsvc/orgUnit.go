// internal/services/authzsvc/orgUnit.go
package authzsvc

import (
	"context"
	"fmt"
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
	var rootIds []string

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

	// collect root ids
	for _, node := range nodeMap {
		if node.ParentId == nil {
			rootIds = append(rootIds, node.Id)
		}
	}

	// build parent-child relationships using pointer references
	for _, node := range nodeMap {
		if node.ParentId == nil {
			continue
		}

		// Debug: print hex comparison when looking up parent
		fmt.Printf("DEBUG: Looking for parent '%s' (hex: %x) for node '%s'\n", *node.ParentId, []byte(*node.ParentId), node.Id)

		parent, ok := nodeMap[*node.ParentId]
		if !ok {
			node.Orphaned = true
			rootIds = append(rootIds, node.Id)
			// Debug: show available parent keys
			fmt.Printf("DEBUG: Parent NOT FOUND! Available keys in nodeMap:\n")
			for k := range nodeMap {
				fmt.Printf("  - '%s' (hex: %x)\n", k, []byte(k))
			}
			continue
		}

		// Build tree using pointer references
		parent.Children = append(parent.Children, *node)
	}

	// Debug: log orphaned nodes
	var orphanedUnits []string
	for _, n := range nodeMap {
		if n.Orphaned {
			orphanedUnits = append(orphanedUnits, fmt.Sprintf("%s (looking for parent: %v)", n.Id, n.ParentId))
		}
	}
	if len(orphanedUnits) > 0 {
		fmt.Printf("DEBUG buildOrgUnitTree: Found %d orphaned nodes:\n", len(orphanedUnits))
		for _, u := range orphanedUnits {
			fmt.Printf("  - %s\n", u)
		}
	}

	// Build result from rootIds - deep copy from nodeMap
	result := make([]OrgUnitNode, 0, len(rootIds))
	for _, id := range rootIds {
		if node, ok := nodeMap[id]; ok {
			// Look up children from nodeMap to get the actual tree structure
			children := make([]OrgUnitNode, 0, len(node.Children))
			for i := range node.Children {
				child := node.Children[i]
				children = append(children, copyNodeRecursive(child))
			}
			result = append(result, OrgUnitNode{
				Id:          node.Id,
				ParentId:    node.ParentId,
				Name:        node.Name,
				Description: node.Description,
				IsRoot:      node.IsRoot,
				Children:    children,
				CreatedAt:   node.CreatedAt,
				UpdatedAt:   node.UpdatedAt,
				Orphaned:    node.Orphaned,
			})
		}
	}
	return result
}

// copyNode recursively copies a node and its children
func copyNodeRecursive(node OrgUnitNode) OrgUnitNode {
	children := make([]OrgUnitNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, copyNodeRecursive(child))
	}
	return OrgUnitNode{
		Id:          node.Id,
		ParentId:    node.ParentId,
		Name:        node.Name,
		Description: node.Description,
		IsRoot:      node.IsRoot,
		Children:    children,
		CreatedAt:   node.CreatedAt,
		UpdatedAt:   node.UpdatedAt,
		Orphaned:    node.Orphaned,
	}
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

	// Get all units to build complete tree
	units, err := s.orgUnitRepo.ListByOrg(ctx, tenantId, orgId)
	if err != nil {
		return nil, err
	}

	// Build complete tree with all levels
	tree := buildOrgUnitTree(units)

	// Build a filtered tree with only root nodes and their level 1 children
	var result []OrgUnitNode
	for _, rootNode := range tree {
		// Keep only root node and its immediate children
		// Set children's children to empty (no grandchildren)
		children := make([]OrgUnitNode, len(rootNode.Children))
		for i, child := range rootNode.Children {
			children[i] = OrgUnitNode{
				Id:          child.Id,
				ParentId:    child.ParentId,
				Name:        child.Name,
				Description: child.Description,
				IsRoot:      child.IsRoot,
				Children:    []OrgUnitNode{}, // Empty - no grandchildren
				CreatedAt:   child.CreatedAt,
				UpdatedAt:   child.UpdatedAt,
				Orphaned:    child.Orphaned,
			}
		}

		result = append(result, OrgUnitNode{
			Id:          rootNode.Id,
			ParentId:    rootNode.ParentId,
			Name:        rootNode.Name,
			Description: rootNode.Description,
			IsRoot:      rootNode.IsRoot,
			Children:    children,
			CreatedAt:   rootNode.CreatedAt,
			UpdatedAt:   rootNode.UpdatedAt,
			Orphaned:    rootNode.Orphaned,
		})
	}

	log.Info().
		Int("unitCount", len(units)).
		Int("rootCount", len(result)).
		Msg("orgUnit tree built")

	return result, nil
}

// GetOrgUnitTreeNode returns a single node with its immediate children (1 level only)
func (s *OrgUnitService) GetOrgUnitTreeNode(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
) (*OrgUnitNode, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"orgUnit.tree.node",
		"authzsvc",
		"GetOrgUnitTreeNode",
	)
	defer end()

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	unitId = strings.TrimSpace(unitId)

	if tenantId == "" || orgId == "" || unitId == "" {
		return nil, ErrUnauthorized
	}

	// Find the requested node
	node, err := s.orgUnitRepo.FindByUnitId(ctx, tenantId, orgId, unitId)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNotFound
	}

	// Find immediate children
	children, err := s.orgUnitRepo.FindChildren(ctx, tenantId, orgId, unitId)
	if err != nil {
		return nil, err
	}

	// Convert children to OrgUnitNode (without grandchildren)
	childNodes := make([]OrgUnitNode, len(children))
	for i, child := range children {
		childNodes[i] = OrgUnitNode{
			Id:          child.UnitId,
			ParentId:    child.ParentUnitId,
			Name:        child.Name,
			Description: child.Description,
			IsRoot:      child.IsRoot,
			Children:    []OrgUnitNode{}, // Empty - only 1 level deep
			CreatedAt:   child.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   child.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
	}

	result := &OrgUnitNode{
		Id:          node.UnitId,
		ParentId:    node.ParentUnitId,
		Name:        node.Name,
		Description: node.Description,
		IsRoot:      node.IsRoot,
		Children:    childNodes,
		CreatedAt:   node.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   node.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}

	log.Info().
		Str("unitId", unitId).
		Int("childCount", len(children)).
		Msg("orgUnit tree node fetched")

	return result, nil
}

// GetOrgUnitsRaw returns raw orgUnits from DB (for debugging)
func (s *OrgUnitService) GetOrgUnitsRaw(
	ctx context.Context,
	tenantId string,
	orgId string,
) ([]authzmod.OrgUnit, error) {
	return s.orgUnitRepo.ListByOrg(ctx, tenantId, orgId)
}

// BuildOrgUnitTree builds tree from raw units (for debugging)
func (s *OrgUnitService) BuildOrgUnitTree(units []authzmod.OrgUnit) []OrgUnitNode {
	return buildOrgUnitTree(units)
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

	// ตรวจ parentId ก่อน insert — ป้องกัน orphan record
	if parentId != nil {
		parent, err := s.orgUnitRepo.FindByUnitId(ctx, tenantId, orgId, *parentId)
		if err != nil {
			return "", err
		}
		if parent == nil {
			return "", ErrInvalidParent
		}
	}

	unit := &authzmod.OrgUnit{
		ID:           primitive.NewObjectID(),
		UnitId:       unitId,
		TenantId:     tenantId,
		WorkspaceId:  orgId,
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

	// Add parent relation if this is a child unit
	if parentId != nil {
		tuples = append(tuples, map[string]interface{}{
			"entity":   map[string]interface{}{"type": "orgUnit", "id": unitId},
			"relation": "parent",
			"subject":  map[string]interface{}{"type": "orgUnit", "id": *parentId},
		})
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
	parentId *string,
	parentIdProvided bool,
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

	// 1️⃣ Update name and description
	if err := s.orgUnitRepo.UpdateMetadata(
		ctx,
		tenantId,
		orgId,
		unitId,
		name,
		description,
		updatedBy,
	); err != nil {
		return err
	}

	// 2️⃣ Handle parentId changes (only if parentId was explicitly provided in request)
	if !parentIdProvided {
		return nil
	}

	oldParentId := unit.ParentUnitId
	parentChanged := false

	if (oldParentId == nil && parentId != nil) ||
		(oldParentId != nil && parentId == nil) ||
		(oldParentId != nil && parentId != nil && *oldParentId != *parentId) {
		parentChanged = true
	}

	if parentChanged {
		// Validate new parent exists (if provided)
		if parentId != nil {
			parent, err := s.orgUnitRepo.FindByUnitId(ctx, tenantId, orgId, *parentId)
			if err != nil {
				return err
			}
			if parent == nil {
				return ErrInvalidParent
			}
		}

		// Delete old parent tuple (if existed)
		if oldParentId != nil {
			_ = s.authzClient.DeleteSpecificTupleWithRelation(
				ctx,
				tenantId,
				"orgUnit",
				unitId,
				"parent",
				"orgUnit",
				*oldParentId,
			)
		}

		// Write new parent tuple (if new parentId provided)
		if parentId != nil {
			tuples := []map[string]interface{}{
				{
					"entity":   map[string]interface{}{"type": "orgUnit", "id": unitId},
					"relation": "parent",
					"subject":  map[string]interface{}{"type": "orgUnit", "id": *parentId},
				},
			}
			_ = s.authzClient.WriteTuples(ctx, tenantId, tuples)
		}

		// Update mongo document
		if err := s.orgUnitRepo.UpdateParent(ctx, tenantId, orgId, unitId, parentId, updatedBy); err != nil {
			return err
		}
	}

	return nil
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
