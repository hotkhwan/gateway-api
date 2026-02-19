// internal/services/authzsvc/orgSelector.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

type OrgSummary struct {
	OrgId       string `json:"orgId"`
	TenantId    string `json:"tenantId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func ListUserOrganizations(ctx context.Context, tenantId string, userId string) ([]OrgSummary, error) {
	log := logger.FromCtx(ctx, "authzsvc", "ListUserOrganizations")

	tenantId = strings.TrimSpace(tenantId)
	userId = strings.TrimSpace(userId)

	if tenantId == "" || userId == "" {
		return nil, ErrUnauthorized
	}
	if strings.TrimSpace(config.CurrentSchemaVersion) == "" {
		return nil, fmt.Errorf("schema version not initialized")
	}

	client := authzgw.NewClient()

	// ✅ important: make lookup consistent with tuples subject id format
	subjectUserId := userId
	// subjectUserId := normalizeUserId(userId)

	log.Info().
		Str("tenantId", tenantId).
		Str("userId", subjectUserId).
		Msg("🔎 looking up organizations from permify")

	orgIds, err := client.LookupOrganizations(ctx, tenantId, subjectUserId)
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

	// ✅ map full fields
	byId := make(map[string]OrgSummary, len(orgs))
	for _, o := range orgs {
		byId[o.OrgId] = OrgSummary{
			OrgId:       o.OrgId,
			TenantId:    o.TenantId,
			Name:        o.Name,
			Description: o.Description,
			CreatedAt:   o.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   o.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
	}

	// ✅ rebuild by permify order
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
		log.Warn().
			Int("missingInMongo", missing).
			Int("orgCount", len(orgIds)).
			Int("mongoCount", len(orgs)).
			Msg("⚠️ permify returned orgIds that are missing in mongo")
	}

	return result, nil
}
