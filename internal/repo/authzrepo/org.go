// internal/repo/authzrepo/org.go
package authzrepo

import (
	"context"
	"klynx/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type OrgRepo struct {
	col *mongo.Collection
}

func NewOrgRepo(db *mongo.Database) *OrgRepo {
	return &OrgRepo{
		col: db.Collection("organizations"),
	}
}

func (r *OrgRepo) Insert(ctx context.Context, org *authzmod.Organization) error {
	_, err := r.col.InsertOne(ctx, org)
	return err
}

func (r *OrgRepo) ExistsByName(ctx context.Context, tenantId, name string) (bool, error) {

	count, err := r.col.CountDocuments(ctx, bson.M{
		"tenantId": tenantId,
		"name":     name,
	})

	return count > 0, err
}

func (r *OrgRepo) MarkSyncError(ctx context.Context, orgId string) error {

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"orgId": orgId},
		bson.M{"$set": bson.M{"syncStatus": "errorSync"}},
	)

	return err
}
