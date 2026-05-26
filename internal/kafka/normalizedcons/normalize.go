// internal/kafka/normalizedcons/normalize.go
package normalizedcons

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// applyTemplate fetches the matching template and applies its field mappings
// to the payload. It returns the mapped fields AND the loaded template so
// callers can reuse it (e.g. for classification rule evaluation) without a
// second Mongo/Redis round-trip.
//
// Returns:
//   - mapped fields (or the raw payload if no templateId / template not found /
//     repo is a test stub that doesn't satisfy *ingestrepo.MappingTemplateRepo —
//     field mapping needs the concrete repo because ingestsvc.NewTemplateMatcher
//     does)
//   - the loaded *ingestmod.MappingTemplate (or nil on miss / fetch error)
func applyTemplate(
	ctx context.Context,
	canonical ingestmod.CanonicalEvent,
	templateId, orgId string,
	repo templateRepoI,
	log zerolog.Logger,
) (map[string]any, *ingestmod.MappingTemplate) {
	if templateId == "" || orgId == "" || repo == nil {
		return canonical.Payload, nil
	}

	tmpl, err := repo.FindById(ctx, orgId, templateId)
	if err != nil {
		log.Warn().
			Str("orgId", orgId).
			Str("templateId", templateId).
			Err(err).
			Msg("[normalize] template not found, using raw payload")
		return canonical.Payload, nil
	}

	// Field mapping requires the concrete *ingestrepo.MappingTemplateRepo
	// because ingestsvc.NewTemplateMatcher panics on a nil/incompatible repo.
	// Tests that inject a stub via templateRepoI skip the mapping step but
	// still get the *MappingTemplate for classification, which is the only
	// behavior Phase 1.0 needs to assert.
	concrete, ok := repo.(*ingestrepo.MappingTemplateRepo)
	if !ok {
		return canonical.Payload, tmpl
	}
	matcher := ingestsvc.NewTemplateMatcher(concrete, log)
	mapped, missing := matcher.ApplyMappings(canonical.Payload, tmpl.Mappings)
	if len(missing) > 0 {
		log.Warn().
			Str("templateId", templateId).
			Strs("missingFields", missing).
			Msg("[normalize] missing required fields in payload")
	}
	return mapped, tmpl
}
