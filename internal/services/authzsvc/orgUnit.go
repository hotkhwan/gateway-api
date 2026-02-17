// internal/services/authzsvc/orgUnit.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
)

func CreateOrgUnit(ctx context.Context, tenantId string, orgId string, name string, parentId *string, createdBy string) (string, error) {
  name = strings.TrimSpace(name)
  if name == "" {
    return "", fmt.Errorf("name is required")
  }

  if parentId == nil || strings.TrimSpace(*parentId) == "" {
    return "", fmt.Errorf("parentId is required (root is created by bootstrap only)")
  }

  repo := authzrepo.NewOrgUnitRepo()

  // ✅ validate parent exists in same tenant+org
  parent, err := repo.FindByUnitId(ctx, tenantId, orgId, *parentId)
  if err != nil {
    return "", fmt.Errorf("parent orgUnit not found")
  }
  _ = parent // just for clarity

  unitId := uuid.NewString()

  unit := &authzmod.OrgUnit{
    UnitId: unitId,
    OrgId: orgId,
    TenantId: tenantId,
    ParentId: parentId,
    Name: name,
    IsRoot: false, // ✅ never true here
    CreatedBy: createdBy,
    CreatedAt: time.Now().UnixMilli(),
  }

  if err := repo.Insert(ctx, unit); err != nil {
    return "", err
  }

  return unitId, nil
}


func GetOrgUnitTree(
	ctx context.Context,
	tenantId string,
	orgId string,
) ([]map[string]interface{}, error) {

	repo := authzrepo.NewOrgUnitRepo()

	units, err := repo.ListByOrg(ctx, tenantId, orgId)
	if err != nil {
		return nil, err
	}

	return buildTree(units), nil
}

func UpdateOrgUnit(
	ctx context.Context,
	unitId string,
	name string,
) error {

	repo := authzrepo.NewOrgUnitRepo()
	return repo.UpdateName(ctx, unitId, name)
}

func DeleteOrgUnit(
	ctx context.Context,
	unitId string,
) error {

	repo := authzrepo.NewOrgUnitRepo()
	return repo.Delete(ctx, unitId)
}

func buildTree(units []authzmod.OrgUnit) []map[string]interface{} {
  nodeMap := make(map[string]map[string]interface{}, len(units))
  roots := make([]map[string]interface{}, 0)

  for _, u := range units {
    nodeMap[u.UnitId] = map[string]interface{}{
      "unitId": u.UnitId,
      "name": u.Name,
      "parentId": u.ParentId,
      "children": []map[string]interface{}{},
    }
  }

  for _, u := range units {
    node := nodeMap[u.UnitId]

    if u.ParentId == nil {
      roots = append(roots, node)
      continue
    }

    parent, ok := nodeMap[*u.ParentId]
    if !ok {
      // ✅ orphan: treat as root (better than disappearing)
      node["orphaned"] = true
      roots = append(roots, node)
      continue
    }

    children := parent["children"].([]map[string]interface{})
    parent["children"] = append(children, node)
  }

  return roots
}

