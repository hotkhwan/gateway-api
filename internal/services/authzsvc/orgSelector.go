// internal/services/authzsvc/orgSelector.go
package authzsvc

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

type OrgSummary struct {
	OrgId string `json:"orgId"`
	Name  string `json:"name"`
}

func ListUserOrganizations(
	ctx context.Context,
	tenantId string,
	userId string,
) ([]OrgSummary, error) {

	log := logger.FromCtx(ctx, "authzsvc", "ListUserOrganizations")

	client := authzgw.NewClient()

	log.Info().
		Str("tenantId", tenantId).
		Str("userId", userId).
		Msg("🔎 looking up organizations from permify")
	if config.CurrentSchemaVersion == "" {
		return nil, fmt.Errorf("schema version not initialized")
	}
	orgIds, err := client.LookupOrganizations(ctx, tenantId, userId)
	if err != nil {
		log.Error().
			Err(err).
			Msg("❌ permify lookup failed")
		return nil, err
	}

	log.Info().
		Int("orgCount", len(orgIds)).
		Msg("✅ permify lookup success")

	if len(orgIds) == 0 {
		return []OrgSummary{}, nil
	}

	orgRepo := authzrepo.NewOrgRepo(config.DB)

	log.Info().
		Msg("🔎 fetching org details from mongo")

	orgs, err := orgRepo.FindByIds(ctx, tenantId, orgIds)
	if err != nil {
		log.Error().
			Err(err).
			Msg("❌ mongo fetch failed")
		return nil, err
	}

	log.Info().
		Int("mongoCount", len(orgs)).
		Msg("✅ mongo fetch success")

	var result []OrgSummary
	for _, o := range orgs {
		result = append(result, OrgSummary{
			OrgId: o.OrgId,
			Name:  o.Name,
		})
	}

	return result, nil
}
