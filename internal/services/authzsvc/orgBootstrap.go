// internal/services/authzsvc/orgBootstrap.go
package authzsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func BootstrapOrganization(
	ctx context.Context,
	tenantId string,
	userId string,
	name string,
) (*authzmod.Organization, error) {

	tenantId = strings.TrimSpace(tenantId)
	userId = strings.TrimSpace(userId)
	name = strings.TrimSpace(name)

	if tenantId == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "tenantId required")
	}
	if userId == "" {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "userId required")
	}
	if name == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "name required")
	}

	repo := authzrepo.NewOrgRepo(config.DB)

	// optional pre-check (ช่วย UX) แต่ไม่ rely on it
	exists, err := repo.ExistsByName(ctx, tenantId, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fiber.NewError(fiber.StatusConflict, "organization name already exists")
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

	// Mongo insert (DB unique index will enforce race condition)
	if err := repo.Insert(ctx, org); err != nil {
		if errors.Is(err, authzrepo.ErrOrgNameAlreadyExists) {
			return nil, fiber.NewError(fiber.StatusConflict, "organization name already exists")
		}
		return nil, err
	}

	// tuples batch
	client := authzgw.NewClient()

	tuples := TupleFactoryOrgBootstrap(orgId, userId) // []map[string]interface{}
	if err := client.WriteTuples(ctx, tenantId, tuples); err != nil {
	_ = repo.MarkSyncError(ctx, orgId)
	return nil, fmt.Errorf("authz sync failed")
	}

	return org, nil
}

