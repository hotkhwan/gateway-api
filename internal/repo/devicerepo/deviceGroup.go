// internal/repo/devicerepo/deviceGroup.go
package devicerepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/devmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const collResourceGroups = "resource_groups"

var (
	ErrNotFound                = errors.New("resource group not found")
	ErrGroupNameAlreadyExists  = errors.New("resource group name already exists in this org")
	ErrCrossOrgAccess          = errors.New("resource group does not belong to this organization")
	ErrCameraNameAlreadyExists = errors.New("camera name already exists in this org")
)

type ResourceGroupRepo struct {
	collection string
}

func NewResourceGroupRepo() *ResourceGroupRepo {
	return &ResourceGroupRepo{collection: collResourceGroups}
}

// ============================================================
// EnsureIndexes
// ============================================================

func (r *ResourceGroupRepo) EnsureIndexes(ctx context.Context) error {
	if err := stomongo.EnsureUniqueIndex(
		ctx,
		r.collection,
		bson.D{
			{Key: "tenantId", Value: 1},
			{Key: "orgId", Value: 1},
			{Key: "name", Value: 1},
		},
		"uq_tenant_org_groupName",
	); err != nil {
		return err
	}

	return stomongo.CreateIndexes(ctx, r.collection, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}}},
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "resourceType", Value: 1}}},
	})
}

// ============================================================
// Insert
// ============================================================

func (r *ResourceGroupRepo) Insert(ctx context.Context, group *devmod.ResourceGroup) error {
	_, err := stomongo.InsertOne(ctx, r.collection, group)
	if err != nil {
		if isDuplicateKeyErr(err) {
			return ErrGroupNameAlreadyExists
		}
		return err
	}
	return nil
}

// ============================================================
// FindByID
// ============================================================

func (r *ResourceGroupRepo) FindByID(ctx context.Context, groupId string) (*devmod.ResourceGroup, error) {
	oid, err := primitive.ObjectIDFromHex(groupId)
	if err != nil {
		return nil, ErrNotFound
	}

	var result devmod.ResourceGroup
	err = stomongo.FindOne(ctx, r.collection, bson.M{"_id": oid}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// ============================================================
// FindByIDAndOrg — cross-org protection
// ============================================================

func (r *ResourceGroupRepo) FindByIDAndOrg(ctx context.Context, groupId, tenantId, orgId string) (*devmod.ResourceGroup, error) {
	group, err := r.FindByID(ctx, groupId)
	if err != nil {
		return nil, err
	}
	if group.TenantID != tenantId || group.OrgID != orgId {
		return nil, ErrCrossOrgAccess
	}
	return group, nil
}

// ============================================================
// ListOptions — filter + pagination + sort
// ============================================================

type ListOptions struct {
	Search       string
	ResourceType string // filter by resourceType ("" = all)
	Page         int
	PerPages     int
	SortField    string
	SortOrder    string // "asc" | "desc"
}

// ============================================================
// List
// ============================================================

func (r *ResourceGroupRepo) List(ctx context.Context, tenantId, orgId string, opts ListOptions) ([]devmod.ResourceGroup, int64, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPages <= 0 {
		opts.PerPages = 10
	}
	if opts.SortField == "" {
		opts.SortField = "createdAt"
	}
	sortVal := -1
	if opts.SortOrder == "asc" {
		sortVal = 1
	}

	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
	}
	if opts.Search != "" {
		filter["name"] = bson.M{"$regex": opts.Search, "$options": "i"}
	}
	if opts.ResourceType != "" {
		filter["resourceType"] = opts.ResourceType
	}

	var results []devmod.ResourceGroup
	if err := stomongo.FindPaginated(ctx, r.collection, filter, stomongo.PaginateOptions{
		Page:    opts.Page,
		PerPage: opts.PerPages,
		Sort:    bson.D{{Key: opts.SortField, Value: sortVal}},
	}, &results); err != nil {
		return nil, 0, err
	}

	total, err := stomongo.Count(ctx, r.collection, filter)
	if err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// ============================================================
// SetSyncStatus
// ============================================================

func (r *ResourceGroupRepo) SetSyncStatus(ctx context.Context, groupId, status string) error {
	oid, err := primitive.ObjectIDFromHex(groupId)
	if err != nil {
		return ErrNotFound
	}
	_, err = stomongo.UpdateOne(ctx, r.collection, bson.M{"_id": oid}, bson.M{
		"syncStatus": status, "updatedAt": time.Now(),
	})
	return err
}

// ============================================================
// Delete — hard delete ลบออกจาก MongoDB ถาวร
// ============================================================

func (r *ResourceGroupRepo) Delete(ctx context.Context, groupId string) error {
	oid, err := primitive.ObjectIDFromHex(groupId)
	if err != nil {
		return ErrNotFound
	}
	res, err := stomongo.DeleteOne(ctx, r.collection, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// helpers
// ============================================================

func isDuplicateKeyErr(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	return false
}
