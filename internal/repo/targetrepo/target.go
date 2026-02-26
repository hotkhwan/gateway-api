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
	return stomongo.EnsureUniqueIndex(ctx, r.collection, bson.D{
		{Key: "tenantId", Value: 1},
		{Key: "orgId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_target_name_per_org")
}

func (r *TargetRepo) ExistsByNameInOrg(ctx context.Context, tenantId, orgId, name, excludeID string) (bool, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"name":     name,
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

func (r *TargetRepo) FindByIDAndOrg(ctx context.Context, targetId, tenantId, orgId string) (*authzmod.DeliveryTarget, error) {
	var result authzmod.DeliveryTarget
	err := stomongo.FindOne(ctx, r.collection, bson.M{
		"targetId": targetId,
		"tenantId": tenantId,
		"orgId":    orgId,
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
	tenantId, orgId, search string,
	page, perPage int,
	sortField, sortOrder string,
) ([]authzmod.DeliveryTarget, int64, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
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

func (r *TargetRepo) Update(ctx context.Context, targetId, tenantId, orgId string, fields bson.M) error {
	fields["updatedAt"] = time.Now().UTC()
	_, err := stomongo.UpdateOne(ctx, r.collection, bson.M{
		"targetId": targetId,
		"tenantId": tenantId,
		"orgId":    orgId,
	}, fields)
	return err
}

func (r *TargetRepo) Delete(ctx context.Context, targetId, tenantId, orgId string) error {
	_, err := stomongo.DeleteOne(ctx, r.collection, bson.M{
		"targetId": targetId,
		"tenantId": tenantId,
		"orgId":    orgId,
	})
	return err
}

// FindEnabledByOrg ใช้สำหรับ distributor worker ดึง active targets ของ org
func (r *TargetRepo) FindEnabledByOrg(ctx context.Context, tenantId, orgId string) ([]authzmod.DeliveryTarget, error) {
	var results []authzmod.DeliveryTarget
	err := stomongo.Find(ctx, r.collection, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"enabled":  true,
	}, nil, &results)
	return results, err
}

// CountByOrg ใช้ตรวจก่อน delete org
func (r *TargetRepo) CountByOrg(ctx context.Context, tenantId, orgId string) (int64, error) {
	return stomongo.Count(ctx, r.collection, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
	})
}
