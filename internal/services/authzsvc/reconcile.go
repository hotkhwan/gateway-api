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

			orgs, err := orgRepo.ListBySyncStatus(ctx, "errorSync")
			if err != nil {
				continue
			}

			client := authzgw.NewClient()

			for _, org := range orgs {
				// ✅ org-only bootstrap tuples
				tuples := TupleFactoryOrgBootstrap(org.OrgId, org.CreatedBy)

				err = client.WriteTuples(ctx, org.TenantId, tuples)
				if err == nil {
					_ = orgRepo.MarkSyncOK(ctx, org.OrgId)
				}
			}
		}
	}()
}
