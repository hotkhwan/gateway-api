// internal/services/authzsvc/org.go
package authzsvc

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"go.mongodb.org/mongo-driver/bson"
)

func UpdateOrg(ctx context.Context, orgID string, name string) error {
	log := logger.WithMeta("authzsvc", "UpdateOrg")

	if name == "" {
		return errors.New("name is required")
	}

	repo := authzrepo.NewOrgRepo(config.DB)

	err := repo.Update(ctx, orgID, bson.M{
		"$set": bson.M{"name": name},
	})
	if err != nil {
		log.Error().Err(err).Str("orgId", orgID).Msg("failed to update org")
		return err
	}

	log.Info().Str("orgId", orgID).Msg("org updated")
	return nil
}

func DeleteOrg(ctx context.Context, orgID string) error {
	log := logger.WithMeta("authzsvc", "DeleteOrg")

	repo := authzrepo.NewOrgRepo(config.DB)

	if err := repo.Delete(ctx, orgID); err != nil {
		log.Error().Err(err).Str("orgId", orgID).Msg("failed to delete org")
		return err
	}

	log.Info().Str("orgId", orgID).Msg("org deleted")
	return nil
}
