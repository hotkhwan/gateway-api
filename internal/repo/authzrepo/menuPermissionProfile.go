// internal/repo/authzrepo/menuPermissionProfile.go
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

var ErrMenuProfileNotFound = errors.New("menu permission profile not found")
var ErrMenuProfileNameExists = errors.New("menu profile name already exists in this org")

type MenuPermissionProfileRepo struct {
	collection string
}

func NewMenuPermissionProfileRepo() *MenuPermissionProfileRepo {
	return &MenuPermissionProfileRepo{collection: "menu_permission_profiles"}
}

func (r *MenuPermissionProfileRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(ctx, r.collection, bson.D{
		{Key: "tenantId", Value: 1},
		{Key: "orgId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_menu_profile_name_per_org")
}

func (r *MenuPermissionProfileRepo) ExistsByNameInOrg(ctx context.Context, tenantId, orgId, name, excludeID string) (bool, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"name":     name,
	}
	if excludeID != "" {
		filter["profileId"] = bson.M{"$ne": excludeID}
	}
	count, err := stomongo.Count(ctx, r.collection, filter)
	return count > 0, err
}

func (r *MenuPermissionProfileRepo) Insert(ctx context.Context, p *authzmod.MenuPermissionProfile) error {
	p.ProfileID = uuid.NewString()
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.OrgUnitIDs == nil {
		p.OrgUnitIDs = []string{}
	}
	if p.MenuIDs == nil {
		p.MenuIDs = []string{}
	}
	if len(p.Relations) == 0 {
		p.Relations = []string{"viewer"}
	}
	_, err := stomongo.InsertOne(ctx, r.collection, p)
	return err
}

func (r *MenuPermissionProfileRepo) FindByIDAndOrg(ctx context.Context, profileId, tenantId, orgId string) (*authzmod.MenuPermissionProfile, error) {
	var result authzmod.MenuPermissionProfile
	err := stomongo.FindOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"orgId":     orgId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrMenuProfileNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *MenuPermissionProfileRepo) List(
	ctx context.Context,
	tenantId, orgId, search string,
	page, perPage int,
	sortField, sortOrder string,
) ([]authzmod.MenuPermissionProfile, int64, error) {
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

	var results []authzmod.MenuPermissionProfile
	pag, err := stomongo.FindWithPagination[authzmod.MenuPermissionProfile](
		ctx, r.collection, filter,
		bson.D{{Key: sortField, Value: sortDir}},
		page, perPage, &results,
	)
	if err != nil {
		return nil, 0, err
	}
	return results, int64(pag.TotalRecords), nil
}

func (r *MenuPermissionProfileRepo) Update(ctx context.Context, profileId, tenantId, orgId string, fields bson.M) error {
	fields["updatedAt"] = time.Now()
	_, err := stomongo.UpdateOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"orgId":     orgId,
	}, fields)
	return err
}

func (r *MenuPermissionProfileRepo) Delete(ctx context.Context, profileId, tenantId, orgId string) error {
	_, err := stomongo.DeleteOne(ctx, r.collection, bson.M{
		"profileId": profileId,
		"tenantId":  tenantId,
		"orgId":     orgId,
	})
	return err
}

// FindActiveByOrgUnitIDs returns active profiles that contain any of the given orgUnitIds (admin endpoint use).
func (r *MenuPermissionProfileRepo) FindActiveByOrgUnitIDs(ctx context.Context, tenantId, orgId string, ouIDs []string) ([]authzmod.MenuPermissionProfile, error) {
	if len(ouIDs) == 0 {
		return []authzmod.MenuPermissionProfile{}, nil
	}
	var results []authzmod.MenuPermissionProfile
	err := stomongo.Find(ctx, r.collection, bson.M{
		"tenantId":   tenantId,
		"orgId":      orgId,
		"status":     true,
		"orgUnitIds": bson.M{"$in": ouIDs},
	}, nil, &results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// FindActiveMenuIDsByOrgUnitIDs returns a deduplicated set of menuIds accessible via the given orgUnits.
// Does not filter by tenantId/orgId — used by introspect where those are not available.
func (r *MenuPermissionProfileRepo) FindActiveMenuIDsByOrgUnitIDs(ctx context.Context, ouIDs []string) ([]string, error) {
	if len(ouIDs) == 0 {
		return []string{}, nil
	}
	var profiles []authzmod.MenuPermissionProfile
	err := stomongo.Find(ctx, r.collection, bson.M{
		"status":     true,
		"orgUnitIds": bson.M{"$in": ouIDs},
	}, nil, &profiles)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, p := range profiles {
		for _, id := range p.MenuIDs {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	return result, nil
}
