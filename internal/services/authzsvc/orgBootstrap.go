// internal/services/authzsvc/orgBootstrap.go
package authzsvc

import (
	"context"
	"fmt"
	"time"

	"klynx/config"
	"klynx/internal/repo/authzrepo"
	"klynx/models/authzmod"

	"github.com/google/uuid"
)

func BootstrapOrganization(
	ctx context.Context,
	tenantId string,
	userId string,
	name string,
) (*authzmod.Organization, error) {

	repo := authzrepo.NewOrgRepo(config.DB)

	// 1️⃣ unique check
	exists, err := repo.ExistsByName(ctx, tenantId, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("organization name already exists")
	}

	now := time.Now().UnixMilli()
	orgId := uuid.NewString()

	org := &authzmod.Organization{
		OrgId:      orgId,
		TenantId:   tenantId,
		Name:       name,
		CreatedBy:  userId,
		CreatedAt:  now,
		SyncStatus: "ok",
	}

	// 2️⃣ Mongo insert
	if err := repo.Insert(ctx, org); err != nil {
		return nil, err
	}

	// 3️⃣ Prepare tuples
	tuples := TupleFactoryOrgBootstrap(orgId, userId)

	// 4️⃣ Write permify (batch)
	if err := WriteTuples(ctx, tenantId, tuples); err != nil {

		repo.MarkSyncError(ctx, orgId)

		return nil, fmt.Errorf("permify sync failed")
	}

	return org, nil
}
