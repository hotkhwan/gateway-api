// internal/services/authzsvc/org.go
package authzsvc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

func UpdateOrg(
	ctx context.Context,
	tenantId string,
	userId string,
	orgId string,
	name string,
	description *string,
) error {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.update",
		"authzsvc",
		"UpdateOrg",
	)
	defer end()

	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}

	repo := authzrepo.NewOrgRepo(config.DB)

	update := bson.M{
		"$set": bson.M{
			"name":      name,
			"updatedBy": userId,
			"updatedAt": time.Now().UTC(),
		},
	}

	if description != nil {
		update["$set"].(bson.M)["description"] = strings.TrimSpace(*description)
	}

	if err := repo.Update(ctx, orgId, update); err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("update failed")
		return err
	}

	log.Info().Str("orgId", orgId).Msg("org updated")
	return nil
}

func DeleteOrg(
	ctx context.Context,
	tenantId string,
	userId string,
	orgId string,
) error {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.delete",
		"authzsvc",
		"DeleteOrg",
	)
	defer end()

	repo := authzrepo.NewOrgRepo(config.DB)

	org, err := repo.FindById(ctx, orgId)
	if err != nil {
		return err
	}

	client := authzgw.NewClient()

	if err := client.DeleteOrgRelationships(ctx, org.TenantId, orgId); err != nil {
		return err
	}

	if err := repo.Delete(ctx, orgId); err != nil {
		return err
	}

	log.Info().Str("orgId", orgId).Msg("org deleted")
	return nil
}
