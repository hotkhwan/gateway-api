// internal/repo/targetrepo/target.go
package targetrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrTargetNotFound = errors.New("delivery target not found")
var ErrTargetNameExists = errors.New("target name already exists in this org")

type TargetRepo struct {
	collection string
}

func NewTargetRepo() *TargetRepo {
	return &TargetRepo{collection: "delivery_targets"}
}

func (r *TargetRepo) EnsureIndexes(ctx context.Context) error {
	// Drop legacy index (orgId → workspaceId rename migration).
	_ = stomongo.DropIndexIfExists(ctx, r.collection, "uq_target_name_per_org")
	return stomongo.EnsureUniqueIndex(ctx, r.collection, bson.D{
		{Key: "tenantId", Value: 1},
		{Key: "workspaceId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_target_name_per_workspace")
}

func (r *TargetRepo) ExistsByNameInOrg(ctx context.Context, tenantId, workspaceId, name, excludeID string) (bool, error) {
	filter := bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
		"name":        name,
	}
	if excludeID != "" {
		filter["targetId"] = bson.M{"$ne": excludeID}
	}
	count, err := stomongo.Count(ctx, r.collection, filter)
	return count > 0, err
}

func (r *TargetRepo) Insert(ctx context.Context, t *authzmod.DeliveryTarget) error {
	t.TargetId = uuid.NewString()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := stomongo.InsertOne(ctx, r.collection, t)
	return err
}

func (r *TargetRepo) FindByIDAndOrg(ctx context.Context, targetId, tenantId, workspaceId string) (*authzmod.DeliveryTarget, error) {
	var result authzmod.DeliveryTarget
	err := stomongo.FindOne(ctx, r.collection, bson.M{
		"targetId":    targetId,
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}
	return &result, nil
}

// FindByIDAndWorkspace looks up a delivery target by targetId scoped to workspace only.
// Used on the delivery path where tenantId may not match the stored value — e.g. the
// klynx-api republish sets event.TenantId to the klynxOrgId UUID while the target in
// Mongo was created with the original tenant string ("klynx"). Workspace is 1-to-1
// with tenant so targetId + workspaceId is sufficient for isolation.
func (r *TargetRepo) FindByIDAndWorkspace(ctx context.Context, targetId, workspaceId string) (*authzmod.DeliveryTarget, error) {
	var result authzmod.DeliveryTarget
	err := stomongo.FindOne(ctx, r.collection, bson.M{
		"targetId":    targetId,
		"workspaceId": workspaceId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *TargetRepo) List(
	ctx context.Context,
	tenantId, workspaceId, search string,
	page, perPage int,
	sortField, sortOrder string,
) ([]authzmod.DeliveryTarget, int64, error) {
	filter := bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
	}
	if search != "" {
		filter["name"] = bson.M{"$regex": search, "$options": "i"}
	}

	sortDir := -1
	if sortOrder == "asc" {
		sortDir = 1
	}
	if sortField == "" {
		sortField = "createdAt"
	}

	var results []authzmod.DeliveryTarget
	pag, err := stomongo.FindWithPagination[authzmod.DeliveryTarget](
		ctx, r.collection, filter,
		bson.D{{Key: sortField, Value: sortDir}},
		page, perPage, &results,
	)
	if err != nil {
		return nil, 0, err
	}
	return results, int64(pag.TotalRecords), nil
}

func (r *TargetRepo) Update(ctx context.Context, targetId, tenantId, workspaceId string, fields bson.M) error {
	fields["updatedAt"] = time.Now().UTC()
	_, err := stomongo.UpdateOne(ctx, r.collection, bson.M{
		"targetId":    targetId,
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
	}, fields)
	return err
}

func (r *TargetRepo) Delete(ctx context.Context, targetId, tenantId, workspaceId string) error {
	_, err := stomongo.DeleteOne(ctx, r.collection, bson.M{
		"targetId":    targetId,
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
	})
	return err
}

// FindEnabledByOrg ใช้สำหรับ distributor worker ดึง active targets ของ workspace
func (r *TargetRepo) FindEnabledByOrg(ctx context.Context, tenantId, workspaceId string) ([]authzmod.DeliveryTarget, error) {
	var results []authzmod.DeliveryTarget
	err := stomongo.Find(ctx, r.collection, bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
		"enabled":     true,
	}, nil, &results)
	return results, err
}

// CountByOrg ใช้ตรวจก่อน delete workspace
func (r *TargetRepo) CountByOrg(ctx context.Context, tenantId, workspaceId string) (int64, error) {
	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
	})
}

// CountByTypeAndOrg returns the number of targets of a specific type in a workspace.
func (r *TargetRepo) CountByTypeAndOrg(ctx context.Context, tenantId, workspaceId, targetType string) (int64, error) {
	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
		"type":        targetType,
	})
}

// CountMessageChannelsByOrg returns the total number of line+discord+telegram targets in a workspace.
func (r *TargetRepo) CountMessageChannelsByOrg(ctx context.Context, tenantId, workspaceId string) (int64, error) {
	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId":    tenantId,
		"workspaceId": workspaceId,
		"type":        bson.M{"$in": []string{"line", "discord", "telegram"}},
	})
}

// HasKlynxTarget returns true when the workspace has at least one enabled mode=klynx target.
// Used by normalizedcons to decide whether to route events via EventBridge.
func (r *TargetRepo) HasKlynxTarget(ctx context.Context, workspaceId string) (bool, error) {
	count, err := stomongo.Count(ctx, r.collection, bson.M{
		"workspaceId": workspaceId,
		"mode":        "klynx",
		"enabled":     true,
	})
	return count > 0, err
}

// FindEnabledKlynxTargets returns all enabled mode=klynx targets for a workspace.
func (r *TargetRepo) FindEnabledKlynxTargets(ctx context.Context, workspaceId string) ([]authzmod.DeliveryTarget, error) {
	var results []authzmod.DeliveryTarget
	err := stomongo.Find(ctx, r.collection, bson.M{
		"workspaceId": workspaceId,
		"mode":        "klynx",
		"enabled":     true,
	}, nil, &results)
	return results, err
}
