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

func (r *OrgUnitRepo) FindRootByOrg(
	ctx context.Context,
	orgId string,
) (*authzmod.OrgUnit, error) {

	var result authzmod.OrgUnit

	err := stomongo.FindOne(
		ctx,
		r.collection,
		bson.M{
			"orgId":  orgId,
			"isRoot": true,
		},
		&result,
	)

	if err != nil {
		return nil, err
	}

	return &result, nil
}
