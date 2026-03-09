// internal/services/ingestsvc/suggestion_apply.go
package ingestsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/cachetemplate"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// tryAutoApplySuggestion checks if exactly one suggestion matches the rawBody.
// If so, converts it to a MappingTemplate, persists to DB (idempotent), and invalidates cache.
// Returns (nil, "") if no single match found.
func (s *IngestService) tryAutoApplySuggestion(ctx context.Context, tenantId, orgId, sourceFamily string, rawBody map[string]any) (*ingestmod.MappingTemplate, string) {
	if s.suggestionSvc == nil {
		return nil, ""
	}
	suggestions := s.suggestionSvc.GetByFamily(sourceFamily)
	var matched *ingestmod.MappingSuggestion
	for _, sg := range suggestions {
		if evalSuggestionMatchRule(sg.MatchRule, rawBody) {
			if matched != nil {
				// More than one suggestion matches — ambiguous, skip auto-apply
				return nil, ""
			}
			matched = sg
		}
	}
	if matched == nil {
		return nil, ""
	}

	tmpl := convertSuggestionToTemplate(matched, orgId)

	// Persist to DB (idempotent — skip if already exists)
	existing, err := s.tmplMatcher.repo.FindById(ctx, orgId, tmpl.TemplateId)
	if err == nil && existing != nil {
		// Template already persisted from a previous ingest — use the DB version
		return existing, matched.ID
	}
	if err != nil && err != ingestrepo.ErrTemplateNotFound {
		// Unexpected DB error — log and fall through with in-memory template
		s.logger.Warn().Err(err).
			Str("orgId", orgId).
			Str("templateId", tmpl.TemplateId).
			Msg("[Ingest] failed to check existing auto template, proceeding with insert")
	}

	// Insert new template
	if insertErr := s.tmplMatcher.repo.Insert(ctx, tmpl); insertErr != nil {
		// Likely duplicate key (race condition) — try fetching again
		if existing2, err2 := s.tmplMatcher.repo.FindById(ctx, orgId, tmpl.TemplateId); err2 == nil {
			return existing2, matched.ID
		}
		s.logger.Error().Err(insertErr).
			Str("orgId", orgId).
			Str("templateId", tmpl.TemplateId).
			Msg("[Ingest] failed to persist auto template")
		// Still return the in-memory template so the event is processed
		return tmpl, matched.ID
	}

	// Invalidate V1 + V2 cache so new template is picked up
	_ = cachetemplate.InvalidateByOrg(ctx, orgId)
	cachetemplate.InvalidateMatchV2ByFamily(ctx, tenantId, orgId, sourceFamily)

	s.logger.Info().
		Str("orgId", orgId).
		Str("templateId", tmpl.TemplateId).
		Str("sourceFamily", sourceFamily).
		Msg("[Ingest] auto template persisted to DB")

	return tmpl, matched.ID
}

// convertSuggestionToTemplate builds a MappingTemplate from a MappingSuggestion.
// Only fieldMappings and matchAll/matchAny are populated.
// Other fields (finalEventType, classificationRules, deliveryTargets, messageTemplates)
// are left empty for the user to configure later via the management API.
func convertSuggestionToTemplate(sg *ingestmod.MappingSuggestion, orgId string) *ingestmod.MappingTemplate {
	mappings := make([]ingestmod.FieldMapping, 0, len(sg.FieldMappings))
	for _, fm := range sg.FieldMappings {
		mappings = append(mappings, ingestmod.FieldMapping{
			SourcePath: fm.SourceField,
			TargetPath: fm.TargetField,
		})
	}

	// Convert suggestion match rules to V2 matchAll/matchAny conditions
	matchAll, matchAny := convertSuggestionMatchRules(sg.MatchRule)

	now := time.Now().UTC()
	return &ingestmod.MappingTemplate{
		TemplateId:   "auto:" + sg.ID,
		OrgId:        orgId,
		SourceFamily: sg.SourceFamily,
		Name:         sg.DisplayName + " (auto)",
		Mappings:     mappings,
		MatchAll:     matchAll,
		MatchAny:     matchAny,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// convertSuggestionMatchRules converts SuggestionMatchRule to V2 MatchCondition slices.
func convertSuggestionMatchRules(rule ingestmod.SuggestionMatchRule) (matchAll, matchAny []ingestmod.MatchCondition) {
	if len(rule.Rules) == 0 {
		return nil, nil
	}

	conditions := make([]ingestmod.MatchCondition, 0, len(rule.Rules))
	for _, r := range rule.Rules {
		op := strings.ToLower(r.Operator)
		switch op {
		case "eq":
			conditions = append(conditions, ingestmod.MatchCondition{
				Field:    "raw." + r.Field,
				Operator: "eq",
				Values:   []string{fmt.Sprintf("%v", r.Value)},
			})
		case "contains":
			conditions = append(conditions, ingestmod.MatchCondition{
				Field:    "raw." + r.Field,
				Operator: "contains",
				Values:   []string{fmt.Sprintf("%v", r.Value)},
			})
		case "exists":
			// V2 MatchCondition doesn't have "exists" operator directly — skip.
			// Template will still match via other conditions.
			continue
		}
	}

	switch strings.ToLower(rule.Type) {
	case "any":
		return nil, conditions
	default: // "all"
		return conditions, nil
	}
}
