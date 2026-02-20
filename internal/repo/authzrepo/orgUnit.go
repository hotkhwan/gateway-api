// internal/repo/authzrepo/orgUnit.go
package authzrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type OrgUnitRepo struct {
	collection string
}

func NewOrgUnitRepo() *OrgUnitRepo {
	return &OrgUnitRepo{
		collection: "org_units", // ✅ snake_case
	}
}

// EnsureIndexes should be called during app bootstrap
func (r *OrgUnitRepo) EnsureIndexes(ctx context.Context) error {

	// 1️⃣ unique unitId per tenant+org
	if err := stomongo.EnsureUniqueIndex(
		ctx,
		r.collection,
		bson.D{
			{Key: "tenantId", Value: 1},
			{Key: "orgId", Value: 1},
			{Key: "unitId", Value: 1},
		},
		"uq_tenant_org_unitId",
	); err != nil {
		return err
	}

	// 2️⃣ index for tree lookup (parentUnitId)
	treeIndex := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "orgId", Value: 1},
				{Key: "parentUnitId", Value: 1},
			},
		},
	}
	if err := stomongo.CreateIndexes(ctx, r.collection, treeIndex); err != nil {
		return err
	}

	// 3️⃣ partial unique: root name must be unique per org
	rootUnique := mongo.IndexModel{
		Keys: bson.D{
			{Key: "tenantId", Value: 1},
			{Key: "orgId", Value: 1},
			{Key: "name", Value: 1},
		},
		Options: options.Index().
			SetName("uq_root_name_per_org").
			SetUnique(true).
			SetPartialFilterExpression(bson.M{
				"isRoot": true,
			}),
	}

	return stomongo.CreateIndexes(ctx, r.collection, []mongo.IndexModel{rootUnique})
}

func (r *OrgUnitRepo) Insert(ctx context.Context, unit *authzmod.OrgUnit) error {
	_, err := stomongo.InsertOne(ctx, r.collection, unit)
	return err
}

func (r *OrgUnitRepo) ListByOrg(ctx context.Context, tenantId, orgId string) ([]authzmod.OrgUnit, error) {
	var result []authzmod.OrgUnit
	err := stomongo.Find(ctx, r.collection, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
	}, nil, &result)
	return result, err
}

func (r *OrgUnitRepo) CountChildren(ctx context.Context, tenantId, orgId, parentUnitId string) (int64, error) {
	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId":     tenantId,
		"orgId":        orgId,
		"parentUnitId": parentUnitId,
	})
}

func (r *OrgUnitRepo) FindChildren(
	ctx context.Context,
	tenantId string,
	orgId string,
	parentUnitId string,
) ([]authzmod.OrgUnit, error) {

	var result []authzmod.OrgUnit

	err := stomongo.Find(
		ctx,
		r.collection,
		bson.M{
			"tenantId":     tenantId,
			"orgId":        orgId,
			"parentUnitId": parentUnitId,
		},
		nil,
		&result,
	)

	return result, err
}

func (r *OrgUnitRepo) UpdateMetadata(
	ctx context.Context,
	tenantId, orgId, unitId,
	name, description, updatedBy string,
) error {

	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"unitId":   unitId,
	}

	setFields := bson.M{
		"name":        name,
		"description": description,
		"updatedBy":   updatedBy,
	}

	_, err := stomongo.UpdateOne(ctx, r.collection, filter, setFields)
	return err
}

func (r *OrgUnitRepo) DeleteByUnitId(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
) error {

	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"unitId":   unitId,
	}

	_, err := stomongo.DeleteOne(ctx, r.collection, filter)
	return err
}

func (r *OrgUnitRepo) FindDescendants(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
) ([]authzmod.OrgUnit, error) {

	all, err := r.ListByOrg(ctx, tenantId, orgId)
	if err != nil {
		return nil, err
	}

	// สร้าง children map
	childrenMap := map[string][]authzmod.OrgUnit{}

	for _, u := range all {
		if u.ParentUnitId != nil {
			childrenMap[*u.ParentUnitId] = append(childrenMap[*u.ParentUnitId], u)
		}
	}

	var result []authzmod.OrgUnit
	queue := []string{unitId}

	// BFS
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, child := range childrenMap[current] {
			result = append(result, child)
			queue = append(queue, child.UnitId)
		}
	}

	return result, nil
}

func (r *OrgUnitRepo) CountByOrg(
	ctx context.Context,
	tenantId string,
	orgId string,
) (int64, error) {

	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
	})
}
