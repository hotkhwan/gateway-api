// internal/repo/authzrepo/permissionProfile.go
package authzrepo

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

var ErrPermProfileNotFound = errors.New("permission profile not found")
var ErrPermProfileNameExists = errors.New("profile name already exists in this org")

type PermissionProfileRepo struct {
	collection string
}

func NewPermissionProfileRepo() *PermissionProfileRepo {
	return &PermissionProfileRepo{collection: "permission_profiles"}
}

func (r *PermissionProfileRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(ctx, r.collection, bson.D{
		{Key: "tenantId", Value: 1},
		{Key: "workspaceId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_perm_profile_name_per_org")
}

func (r *PermissionProfileRepo) ExistsByNameInOrg(ctx context.Context, tenantId, orgId, name, excludeID string) (bool, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"workspaceId": orgId,
		"name":     name,
	}
	if excludeID != "" {
		filter["profileId"] = bson.M{"$ne": excludeID}
	}
	count, err := stomongo.Count(ctx, r.collection, filter)
	return count > 0, err
}

func (r *PermissionProfileRepo) Insert(ctx context.Context, p *authzmod.PermissionProfile) error {
	p.ProfileID = uuid.NewString()
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Relations == nil {
		p.Relations = []string{}
	}
	if p.OrgUnitIDs == nil {
		p.OrgUnitIDs = []string{}
	}
	if p.ResourceGroupIDs == nil {
		p.ResourceGroupIDs = []string{}
	}
	_, err := stomongo.InsertOne(ctx, r.collection, p)
	return err
}

func (r *PermissionProfileRepo) FindByIDAndOrg(ctx context.Context, profileId, tenantId, orgId string) (*authzmod.PermissionProfile, error) {
	var result authzmod.PermissionProfile
	err := stomongo.FindOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"workspaceId":    orgId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrPermProfileNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *PermissionProfileRepo) List(
	ctx context.Context,
	tenantId, orgId, search string,
	page, perPage int,
	sortField, sortOrder string,
) ([]authzmod.PermissionProfile, int64, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"workspaceId": orgId,
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

	var results []authzmod.PermissionProfile
	pag, err := stomongo.FindWithPagination(
		ctx, r.collection, filter,
		bson.D{{Key: sortField, Value: sortDir}},
		page, perPage, &results,
	)
	if err != nil {
		return nil, 0, err
	}
	return results, int64(pag.TotalRecords), nil
}

func (r *PermissionProfileRepo) Update(
	ctx context.Context,
	profileId, tenantId, orgId string,
	fields bson.M,
) error {
	fields["updatedAt"] = time.Now()
	_, err := stomongo.UpdateOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"workspaceId":    orgId,
	}, fields)
	return err
}

func (r *PermissionProfileRepo) Delete(ctx context.Context, profileId, tenantId, orgId string) error {
	_, err := stomongo.DeleteOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"workspaceId":    orgId,
	})
	return err
}

// FindActiveByOrgUnitIDs returns active profiles that contain any of the given orgUnitIds.
func (r *PermissionProfileRepo) FindActiveByOrgUnitIDs(ctx context.Context, tenantId, orgId string, ouIDs []string) ([]authzmod.PermissionProfile, error) {
	if len(ouIDs) == 0 {
		return []authzmod.PermissionProfile{}, nil
	}
	var results []authzmod.PermissionProfile
	err := stomongo.Find(ctx, r.collection, bson.M{
		"tenantId":   tenantId,
		"workspaceId":     orgId,
		"status":     true,
		"orgUnitIds": bson.M{"$in": ouIDs},
	}, nil, &results)
	if err != nil {
		return nil, err
	}
	return results, nil
}
