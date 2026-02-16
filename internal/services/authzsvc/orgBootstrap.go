// internal/services/authzsvc/orgBootstrap.go
package authzsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"

	"github.com/google/uuid"
)

func BootstrapOrganization(
	ctx context.Context,
	tenantId string,
	userId string,
	name string,
) (*authzmod.Organization, error) {

	now := time.Now().UnixMilli()
	orgId := uuid.NewString()
	rootUnitId := uuid.NewString()

	org := &authzmod.Organization{
		OrgId:      orgId,
		TenantId:   tenantId,
		Name:       name,
		CreatedBy:  userId,
		CreatedAt:  now,
		SyncStatus: "pending",
	}

	rootUnit := &authzmod.OrgUnit{
		UnitId:    rootUnitId,
		OrgId:     orgId,
		TenantId:  tenantId,
		Name:      "root",
		IsRoot:    true,
		CreatedBy: userId,
		CreatedAt: now,
	}

	orgRepo := authzrepo.NewOrgRepo(config.DB)
	unitRepo := authzrepo.NewOrgUnitRepo()

	// 1️⃣ Insert Mongo first
	if err := orgRepo.Insert(ctx, org); err != nil {
		return nil, err
	}

	if err := unitRepo.Insert(ctx, rootUnit); err != nil {
		return nil, err
	}

	// 2️⃣ Write tuples
	client := authzgw.NewClient()
	tuples := TupleFactoryOrgBootstrap(orgId, rootUnitId, userId)

	if err := client.WriteTuples(ctx, tenantId, tuples); err != nil {
		_ = orgRepo.MarkSyncError(ctx, orgId)
		return nil, fmt.Errorf("authz sync failed: %w", err)
	}

	_ = orgRepo.MarkSyncOK(ctx, orgId)

	return org, nil
}
