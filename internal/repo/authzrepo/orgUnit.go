// internal/repo/authzrepo/orgUnit.go
package authzrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

type OrgUnitRepo struct {
	collection string
}

func NewOrgUnitRepo() *OrgUnitRepo {
	return &OrgUnitRepo{
		collection: "orgUnits",
	}
}

func (r *OrgUnitRepo) Insert(ctx context.Context, unit *authzmod.OrgUnit) error {
	_, err := stomongo.InsertOne(ctx, r.collection, unit)
	return err
}

func (r *OrgUnitRepo) ListByOrg(
	ctx context.Context,
	tenantId string,
	orgId string,
) ([]authzmod.OrgUnit, error) {

	var result []authzmod.OrgUnit

	err := stomongo.Find(
		ctx,
		r.collection,
		bson.M{
			"tenantId": tenantId,
			"orgId":    orgId,
		},
		nil,
		&result,
	)

	return result, err
}

func (r *OrgUnitRepo) Delete(
	ctx context.Context,
	unitId string,
) error {

	_, err := stomongo.DeleteOne(
		ctx,
		r.collection,
		bson.M{"unitId": unitId},
	)

	return err
}
