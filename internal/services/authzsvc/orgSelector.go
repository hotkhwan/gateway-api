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

func ListUserOrganizations(ctx context.Context, tenantId string, userId string) ([]OrgSummary, error) {
  log := logger.FromCtx(ctx, "authzsvc", "ListUserOrganizations")

  client := authzgw.NewClient()

  log.Info().Str("tenantId", tenantId).Str("userId", userId).Msg("🔎 looking up organizations from permify")
  if config.CurrentSchemaVersion == "" {
    return nil, fmt.Errorf("schema version not initialized")
  }

  orgIds, err := client.LookupOrganizations(ctx, tenantId, userId)
  if err != nil {
    log.Error().Err(err).Msg("❌ permify lookup failed")
    return nil, err
  }

  log.Info().Int("orgCount", len(orgIds)).Msg("✅ permify lookup success")
  if len(orgIds) == 0 {
    return []OrgSummary{}, nil
  }

  orgRepo := authzrepo.NewOrgRepo(config.DB)
  orgs, err := orgRepo.FindByIds(ctx, tenantId, orgIds)
  if err != nil {
    log.Error().Err(err).Msg("❌ mongo fetch failed")
    return nil, err
  }

  // ✅ map + rebuild by permify order
  byId := make(map[string]OrgSummary, len(orgs))
  for _, o := range orgs {
    byId[o.OrgId] = OrgSummary{OrgId: o.OrgId, Name: o.Name}
  }

  result := make([]OrgSummary, 0, len(orgIds))
  missing := 0
  for _, id := range orgIds {
    if v, ok := byId[id]; ok {
      result = append(result, v)
    } else {
      missing++
    }
  }

  if missing > 0 {
    log.Warn().Int("missingInMongo", missing).Int("orgCount", len(orgIds)).Int("mongoCount", len(orgs)).
      Msg("⚠️ permify returned orgIds that are missing in mongo")
  }

  return result, nil
}

