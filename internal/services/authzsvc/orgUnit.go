// internal/services/authzsvc/orgUnit.go
package authzsvc

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

func GetOrgUnitTree(
	ctx context.Context,
	tenantId string,
	orgId string,
) ([]OrgUnitNode, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
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

	repo := authzrepo.NewOrgUnitRepo()

	units, err := repo.ListByOrg(ctx, tenantId, orgId)
	if err != nil {
		return nil, err
	}

	tree := buildOrgUnitTree(units)

	log.Info().
		Int("unitCount", len(units)).
		Int("rootCount", len(tree)).
		Msg("orgUnit tree loaded")

	return tree, nil
}

func CreateOrgUnit(
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
		"github.com/hotkhwan/gateway-api/authzsvc",
		"orgUnit.create",
		"authzsvc",
		"CreateOrgUnit",
	)
	defer end()

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	name = strings.TrimSpace(name)
	createdBy = strings.TrimSpace(createdBy)

	if tenantId == "" || orgId == "" || createdBy == "" {
		return "", ErrUnauthorized
	}
	if name == "" {
		return "", ErrBadRequest
	}

	repo := authzrepo.NewOrgUnitRepo()

	var normalizedParentId *string
	if parentId != nil {
		v := strings.TrimSpace(*parentId)
		if v != "" {
			normalizedParentId = &v
		}
	}

	isRoot := normalizedParentId == nil

	// 🔍 validate parent
	if !isRoot {
		parent, err := repo.FindByUnitId(ctx, tenantId, orgId, *normalizedParentId)
		if err != nil || parent == nil {
			return "", ErrInvalidParent
		}
	}

	unitId := uuid.NewString()
	now := time.Now().UTC()

	unit := &authzmod.OrgUnit{
		ID:           primitive.NewObjectID(),
		UnitId:       unitId,
		TenantId:     tenantId,
		OrgId:        orgId,
		ParentUnitId: normalizedParentId,
		Name:         name,
		Description:  description,
		IsRoot:       isRoot,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedBy:    createdBy,
		UpdatedAt:    now,
	}

	// 1️⃣ Insert Mongo first
	if err := repo.Insert(ctx, unit); err != nil {
		return "", err
	}

	// 2️⃣ Prepare Permify tuples
	client := authzgw.NewClient()

	tuples := []map[string]interface{}{
		{
			"entity": map[string]interface{}{
				"type": "orgUnit",
				"id":   unitId,
			},
			"relation": "parentOrg",
			"subject": map[string]interface{}{
				"type": "organization",
				"id":   orgId,
			},
		},
	}

	// parent relation (if not root)
	if normalizedParentId != nil {
		tuples = append(tuples, map[string]interface{}{
			"entity": map[string]interface{}{
				"type": "orgUnit",
				"id":   unitId,
			},
			"relation": "parent",
			"subject": map[string]interface{}{
				"type": "orgUnit",
				"id":   *normalizedParentId,
			},
		})
	}

	// creator auto member
	tuples = append(tuples, map[string]interface{}{
		"entity": map[string]interface{}{
			"type": "orgUnit",
			"id":   unitId,
		},
		"relation": "member",
		"subject": map[string]interface{}{
			"type": "user",
			"id":   createdBy, // rest client จะ normalize เป็น user:<uuid>
		},
	})

	// 3️⃣ Write Permify
	if err := client.WriteTuples(ctx, tenantId, tuples); err != nil {

		// ❗ rollback mongo (atomic safety)
		_ = repo.DeleteByUnitId(ctx, tenantId, orgId, unitId)

		return "", err
	}

	log.Info().
		Str("unitId", unitId).
		Bool("isRoot", isRoot).
		Msg("orgUnit created with permify tuples")

	return unitId, nil
}

func UpdateOrgUnitName(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
	name string,
	description string,
	updatedBy string,
) error {

	ctx, end, _ := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"orgUnit.updateName",
		"authzsvc",
		"UpdateOrgUnitName",
	)
	defer end()

	repo := authzrepo.NewOrgUnitRepo()

	unit, err := repo.FindByUnitId(ctx, tenantId, orgId, unitId)
	if err != nil || unit == nil {
		return ErrNotFound
	}

	// ✅ root allowed to update metadata

	return repo.UpdateMetadata(
		ctx,
		tenantId,
		orgId,
		unitId,
		name,
		description,
		updatedBy,
	)
}

func DeleteOrgUnit(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
	deletedBy string,
) error {

	ctx, end, _ := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"orgUnit.delete",
		"authzsvc",
		"DeleteOrgUnit",
	)
	defer end()

	repo := authzrepo.NewOrgUnitRepo()

	unit, err := repo.FindByUnitId(ctx, tenantId, orgId, unitId)
	if err != nil || unit == nil {
		return ErrNotFound
	}
	if unit.IsRoot {
		return ErrRootImmutable
	}

	childCount, err := repo.CountChildren(ctx, tenantId, orgId, unitId)
	if err != nil {
		return err
	}
	if childCount > 0 {
		return ErrHasChildren
	}

	return repo.DeleteByUnitId(ctx, tenantId, orgId, unitId)
}

func buildOrgUnitTree(units []authzmod.OrgUnit) []OrgUnitNode {

	nodeMap := make(map[string]*OrgUnitNode)
	roots := []*OrgUnitNode{}

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

	for _, u := range units {

		node := nodeMap[u.UnitId]

		if u.ParentUnitId == nil {
			roots = append(roots, node)
			continue
		}

		parent := nodeMap[*u.ParentUnitId]
		if parent == nil {
			node.Orphaned = true
			roots = append(roots, node)
			continue
		}

		parent.Children = append(parent.Children, *node)
	}

	sort.Slice(roots, func(i, j int) bool {
		return strings.ToLower(roots[i].Name) <
			strings.ToLower(roots[j].Name)
	})

	out := make([]OrgUnitNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, *r)
	}

	return out
}
