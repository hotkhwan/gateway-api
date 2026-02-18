// internal/services/authzsvc/orgBootstrap.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
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

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	now := time.Now().UnixMilli()
	orgId := uuid.NewString()
	rootUnitId := uuid.NewString()

	orgRepo := authzrepo.NewOrgRepo(config.DB)
	unitRepo := authzrepo.NewOrgUnitRepo()

	// ✅ Guard: exists-by-name (fast fail)
	exists, err := orgRepo.ExistsByName(ctx, tenantId, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, authzrepo.ErrOrgNameAlreadyExists
	}

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

	// 1️⃣ Insert Mongo first (unique index is final safety net)
	if err := orgRepo.Insert(ctx, org); err != nil {
		return nil, err // Insert() already maps dup-key -> ErrOrgNameAlreadyExists
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

