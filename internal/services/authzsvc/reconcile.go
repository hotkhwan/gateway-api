// internal/services/authzsvc/reconcile.go
package authzsvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

func StartReconcileWorker() {

	go func() {

		for {
			time.Sleep(30 * time.Second)

			ctx := context.Background()

			orgRepo := authzrepo.NewOrgRepo(config.DB)
			unitRepo := authzrepo.NewOrgUnitRepo()

			orgs, err := orgRepo.ListBySyncStatus(ctx, "errorSync")
			if err != nil {
				continue
			}

			for _, org := range orgs {

				root, err := unitRepo.FindRootByOrg(ctx, org.OrgId)
				if err != nil {
					continue
				}

				tuples := TupleFactoryOrgBootstrap(
					org.OrgId,
					root.UnitId,
					org.CreatedBy,
				)

				client := authzgw.NewClient()
				err = client.WriteTuples(ctx, org.TenantId, tuples)
				if err == nil {
					_ = orgRepo.MarkSyncOK(ctx, org.OrgId)
				}
			}
		}
	}()
}
