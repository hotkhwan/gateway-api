// internal/kafka/normalizedcons/normalize.go
package normalizedcons

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// applyTemplate fetches the matching template and applies its field mappings to the payload.
// Returns the mapped fields, or the raw payload if no templateId is provided / template not found.
func applyTemplate(
	ctx context.Context,
	canonical ingestmod.CanonicalEvent,
	templateId, orgId string,
	repo *ingestrepo.MappingTemplateRepo,
	log zerolog.Logger,
) map[string]any {
	if templateId == "" || orgId == "" {
		return canonical.Payload
	}

	tmpl, err := repo.FindById(ctx, orgId, templateId)
	if err != nil {
		log.Warn().
			Str("orgId", orgId).
			Str("templateId", templateId).
			Err(err).
			Msg("[normalize] template not found, using raw payload")
		return canonical.Payload
	}

	matcher := ingestsvc.NewTemplateMatcher(repo, log)
	mapped, missing := matcher.ApplyMappings(canonical.Payload, tmpl.Mappings)
	if len(missing) > 0 {
		log.Warn().
			Str("templateId", templateId).
			Strs("missingFields", missing).
			Msg("[normalize] missing required fields in payload")
	}
	return mapped
}
