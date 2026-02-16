// internal/services/authzsvc/sync.go
package authzsvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

func InitialSyncRelationships(ctx context.Context, tenantId string) error {

	orgRepo := authzrepo.NewOrgRepo(config.DB)
	unitRepo := authzrepo.NewOrgUnitRepo()

	orgs, err := orgRepo.ListAll(ctx, tenantId)
	if err != nil {
		return err
	}

	client := authzgw.NewClient()

	for _, org := range orgs {

		root, err := unitRepo.FindRootByOrg(ctx, tenantId, org.OrgId)
		if err != nil {
			continue
		}

		tuples := TupleFactoryOrgBootstrap(org.OrgId, root.UnitId, org.CreatedBy)

		_ = client.WriteTuples(ctx, tenantId, tuples)
	}

	return nil
}
