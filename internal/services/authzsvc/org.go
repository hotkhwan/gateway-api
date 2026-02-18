// internal/services/authzsvc/org.go
package authzsvc

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

func UpdateOrg(ctx context.Context, orgID string, name string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.UpdateOrg",
		"authzsvc", "UpdateOrg",
	)
	defer end()

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
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.DeleteOrg",
		"authzsvc", "DeleteOrg",
	)
	defer end()

	log.Info().Str("orgId", orgID).Msg("🗑️ starting delete org")

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// ── 1. หา org เพื่อเอา tenantId ──────────────────────────────────────
	log.Info().Str("orgId", orgID).Msg("🔍 [1/3] fetching org from mongo")

	repo := authzrepo.NewOrgRepo(config.DB)
	org, err := repo.FindById(ctx, orgID)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgID).Msg("❌ [1/3] org not found in mongo")
		return err
	}

	log.Info().
		Str("orgId", orgID).
		Str("tenantId", org.TenantId).
		Msg("✅ [1/3] org found")

	// ── 2. ลบ Permify tuples ก่อน ─────────────────────────────────────────
	log.Info().
		Str("orgId", orgID).
		Str("tenantId", org.TenantId).
		Msg("🔐 [2/3] deleting permify tuples")

	client := authzgw.NewClient()
	if err := client.DeleteOrgRelationships(ctx, org.TenantId, orgID); err != nil {
		log.Error().
			Err(err).
			Str("orgId", orgID).
			Str("tenantId", org.TenantId).
			Msg("❌ [2/3] permify delete tuples failed — aborting, mongo NOT touched")
		return err
	}

	log.Info().
		Str("orgId", orgID).
		Str("tenantId", org.TenantId).
		Msg("✅ [2/3] permify tuples deleted")

	// ── 3. ลบ Mongo ───────────────────────────────────────────────────────
	log.Info().Str("orgId", orgID).Msg("🗄️ [3/3] deleting org from mongo")

	if err := repo.Delete(ctx, orgID); err != nil {
		log.Error().Err(err).Str("orgId", orgID).Msg("❌ [3/3] mongo delete failed (WARNING: permify already deleted!)")
		return err
	}

	log.Info().Str("orgId", orgID).Msg("✅ [3/3] org deleted from mongo")
	log.Info().Str("orgId", orgID).Msg("🎉 delete org completed")

	return nil
}