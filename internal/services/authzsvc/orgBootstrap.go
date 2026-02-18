// internal/services/authzsvc/orgBootstrap.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
)

func BootstrapOrganization(
	ctx context.Context,
	tenantId, userId, name string,
	description *string,
) (*authzmod.Organization, error) {

	tenantId = strings.TrimSpace(tenantId)
	userId = strings.TrimSpace(userId)
	name = strings.TrimSpace(name)

	if tenantId == "" || userId == "" {
		return nil, ErrUnauthorized
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	now := time.Now().UTC()
	orgId := uuid.NewString()

	var desc string
	if description != nil {
		desc = strings.TrimSpace(*description)
	}

	org := &authzmod.Organization{
		OrgId:       orgId,
		TenantId:    tenantId,
		Name:        name,
		Description: desc,
		CreatedBy:   userId,
		CreatedAt:   now,
		UpdatedBy:   userId,
		UpdatedAt:   now,
		SyncStatus:  "pending",
	}

	repo := authzrepo.NewOrgRepo(config.DB)

	if err := repo.Insert(ctx, org); err != nil {
		if err == authzrepo.ErrOrgNameAlreadyExists {
			return nil, err
		}
		return nil, err
	}

	client := authzgw.NewClient()
	tuples := TupleFactoryOrgBootstrap(orgId, userId)

	if err := client.WriteTuples(ctx, tenantId, tuples); err != nil {
		_ = repo.MarkSyncError(ctx, orgId)
		return nil, err
	}

	_ = repo.MarkSyncOK(ctx, orgId)

	return org, nil
}
