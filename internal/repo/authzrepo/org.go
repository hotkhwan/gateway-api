// internal/repo/authzrepo/org.go
package authzrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrOrgNameAlreadyExists = errors.New("organization name already exists")

type OrgRepo struct {
	col *mongo.Collection
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

func (r *OrgRepo) ListBySyncStatus(
	ctx context.Context,
	status string,
) ([]authzmod.Organization, error) {

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

func (r *OrgRepo) FindByIds(
	ctx context.Context,
	tenantId string,
	orgIds []string,
) ([]authzmod.Organization, error) {

	var result []authzmod.Organization

	err := stomongo.Find(
		ctx,
		"organizations",
		bson.M{
			"tenantId": tenantId,
			"orgId": bson.M{
				"$in": orgIds,
			},
		},
		nil,
		&result,
	)

	return result, err
}
