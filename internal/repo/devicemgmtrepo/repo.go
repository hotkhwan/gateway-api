// internal/repo/devicemgmtrepo/repo.go
package devicemgmtrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colDeviceMgmt = "device_management"

var ErrNotFound = errors.New("device management record not found")

type DeviceManagementRepo struct{}

func NewDeviceManagementRepo() *DeviceManagementRepo {
	return &DeviceManagementRepo{}
}

func (r *DeviceManagementRepo) Insert(ctx context.Context, d *ingestmod.DeviceManagement) error {
	_, err := stomongo.InsertOne(ctx, colDeviceMgmt, d)
	return err
}

func (r *DeviceManagementRepo) FindById(ctx context.Context, tenantId, orgId, deviceMgmtId string) (*ingestmod.DeviceManagement, error) {
	var result ingestmod.DeviceManagement
	err := stomongo.FindOne(ctx, colDeviceMgmt, bson.M{
		"tenantId":     tenantId,
		"orgId":        orgId,
		"deviceMgmtId": deviceMgmtId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *DeviceManagementRepo) FindByEntity(ctx context.Context, tenantId, orgId, sourceFamily, entityType, entityId string) (*ingestmod.DeviceManagement, error) {
	var result ingestmod.DeviceManagement
	err := stomongo.FindOne(ctx, colDeviceMgmt, bson.M{
		"tenantId":     tenantId,
		"orgId":        orgId,
		"sourceFamily": sourceFamily,
		"entityType":   entityType,
		"entityId":     entityId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *DeviceManagementRepo) List(ctx context.Context, tenantId, orgId string, page, perPage int) ([]*ingestmod.DeviceManagement, error) {
	var result []*ingestmod.DeviceManagement
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * perPage)).
		SetLimit(int64(perPage))
	err := stomongo.Find(ctx, colDeviceMgmt, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
	}, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *DeviceManagementRepo) Update(ctx context.Context, tenantId, orgId, deviceMgmtId string, setFields bson.M) error {
	res, err := stomongo.UpdateOne(ctx, colDeviceMgmt, bson.M{
		"tenantId":     tenantId,
		"orgId":        orgId,
		"deviceMgmtId": deviceMgmtId,
	}, setFields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertByBusinessKey performs an idempotent upsert keyed on (tenantId, orgId, sourceFamily, entityType, entityId).
// Returns "inserted", "updated", or "skipped" depending on the outcome.
func (r *DeviceManagementRepo) UpsertByBusinessKey(ctx context.Context, d *ingestmod.DeviceManagement) (string, error) {
	filter := bson.M{
		"tenantId":     d.TenantId,
		"orgId":        d.OrgId,
		"sourceFamily": d.SourceFamily,
		"entityType":   d.EntityType,
		"entityId":     d.EntityId,
	}

	setFields := bson.M{
		"deviceId":    d.DeviceId,
		"name":        d.Name,
		"description": d.Description,
		"lat":         d.Lat,
		"lng":         d.Lng,
		"site":        d.Site,
		"zone":        d.Zone,
	}
	onInsert := bson.M{
		"deviceMgmtId": d.DeviceMgmtId,
		"createdAt":    d.CreatedAt,
	}

	res, err := stomongo.UpsertByFilter(ctx, colDeviceMgmt, filter, setFields, onInsert)
	if err != nil {
		return "", err
	}
	if res.UpsertedCount > 0 {
		return "inserted", nil
	}
	if res.ModifiedCount > 0 {
		return "updated", nil
	}
	return "skipped", nil
}

// ListForExport returns all records for a given tenant/org scope with optional filters.
func (r *DeviceManagementRepo) ListForExport(ctx context.Context, tenantId, orgId, sourceFamily, entityType string) ([]ingestmod.DeviceManagement, error) {
	filter := bson.M{"tenantId": tenantId, "orgId": orgId}
	if sourceFamily != "" {
		filter["sourceFamily"] = sourceFamily
	}
	if entityType != "" {
		filter["entityType"] = entityType
	}

	opts := options.Find().SetSort(bson.D{{Key: "sourceFamily", Value: 1}, {Key: "entityType", Value: 1}, {Key: "entityId", Value: 1}})
	var result []ingestmod.DeviceManagement
	err := stomongo.Find(ctx, colDeviceMgmt, filter, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CountByFilter returns the count of records matching the given scope.
func (r *DeviceManagementRepo) CountByFilter(ctx context.Context, tenantId, orgId string) (int64, error) {
	return stomongo.Count(ctx, colDeviceMgmt, bson.M{"tenantId": tenantId, "orgId": orgId})
}

func (r *DeviceManagementRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, colDeviceMgmt, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "deviceMgmtId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "sourceFamily", Value: 1}, {Key: "entityType", Value: 1}, {Key: "entityId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
}
