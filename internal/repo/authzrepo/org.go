// internal/repo/authzrepo/org.go
package authzrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)
var ErrOrgNameAlreadyExists = errors.New("organization name already exists")

type OrgRepo struct {
	col *mongo.Collection
}

func (r *OrgUnitRepo) FindByUnitId(ctx context.Context, tenantId string, orgId string, unitId string) (*authzmod.OrgUnit, error) {
  var result authzmod.OrgUnit
  err := stomongo.FindOne(ctx, r.collection, bson.M{
    "tenantId": tenantId,
    "orgId": orgId,
    "unitId": unitId,
  }, &result)
  if err != nil {
    return nil, err
  }
  return &result, nil
}

func (r *OrgUnitRepo) UpdateName(ctx context.Context, unitId string, name string) error {
  _, err := stomongo.UpdateOne(
    ctx,
    r.collection,
    bson.M{"unitId": unitId},
    bson.M{"$set": bson.M{"name": name}}, // ✅ FIX
  )
  return err
}

func NewOrgRepo(db *mongo.Database) *OrgRepo {
	return &OrgRepo{col: db.Collection("organizations")}
}

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

// ✅ Call once at bootstrap
func (r *OrgRepo) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "name", Value: 1},
			},
			Options: options.Index().
				SetName("uniq_tenantId_name").
				SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "orgId", Value: 1},
			},
			Options: options.Index().
				SetName("uniq_tenantId_orgId").
				SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "syncStatus", Value: 1},
			},
			Options: options.Index().
				SetName("idx_syncStatus"),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, models)
	return err
}

func (r *OrgRepo) Insert(ctx context.Context, org *authzmod.Organization) error {
	_, err := r.col.InsertOne(ctx, org)
	if err != nil {
		if isDuplicateKeyErr(err) {
			return ErrOrgNameAlreadyExists
		}
		return err
	}
	return nil
}

func (r *OrgRepo) ExistsByName(ctx context.Context, tenantId, name string) (bool, error) {
	count, err := r.col.CountDocuments(ctx, bson.M{
		"tenantId": tenantId,
		"name":     name,
	})
	return count > 0, err
}

func (r *OrgRepo) MarkSyncError(ctx context.Context, orgId string) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"orgId": orgId}, bson.M{
		"$set": bson.M{"syncStatus": "errorSync"},
	})
	return err
}

func (r *OrgRepo) MarkSyncOK(ctx context.Context, orgId string) error {
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"orgId": orgId},
		bson.M{
			"$set": bson.M{"syncStatus": "ok"},
		},
	)
	return err
}

func (r *OrgRepo) ListAll(ctx context.Context, tenantId string) ([]authzmod.Organization, error) {
	var result []authzmod.Organization
	err := stomongo.Find(
		ctx,
		"organizations",
		bson.M{"tenantId": tenantId},
		nil,
		&result,
	)
	return result, err
}

func (r *OrgRepo) ListBySyncStatus(ctx context.Context, status string) ([]authzmod.Organization, error) {
	var result []authzmod.Organization
	err := stomongo.Find(
		ctx,
		"organizations",
		bson.M{"syncStatus": status},
		nil,
		&result,
	)
	return result, err
}

func (r *OrgRepo) FindById(ctx context.Context, orgId string) (*authzmod.Organization, error) {
    var org authzmod.Organization
    err := r.col.FindOne(ctx, bson.M{"orgId": orgId}).Decode(&org)
    if err != nil {
        return nil, err
    }
    return &org, nil
}

func (r *OrgRepo) FindByIds(ctx context.Context, tenantId string, orgIds []string) ([]authzmod.Organization, error) {
	var result []authzmod.Organization
	err := stomongo.Find(
		ctx,
		"organizations",
		bson.M{
			"tenantId": tenantId,
			"orgId": bson.M{"$in": orgIds},
		},
		nil,
		&result,
	)
	return result, err
}

func (r *OrgRepo) Update(ctx context.Context, orgId string, update bson.M) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"orgId": orgId}, update)
	return err
}

func (r *OrgRepo) Delete(ctx context.Context, orgId string) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"orgId": orgId})
	return err
}

func (r *OrgUnitRepo) FindRootByOrg(
	ctx context.Context,
	tenantId string,
	orgId string,
) (*authzmod.OrgUnit, error) {

	var result authzmod.OrgUnit

	err := stomongo.FindOne(
		ctx,
		r.collection,
		bson.M{
			"tenantId": tenantId,
			"orgId":    orgId,
			"isRoot":   true,
		},
		&result,
	)

	if err != nil {
		return nil, err
	}

	return &result, nil
}
