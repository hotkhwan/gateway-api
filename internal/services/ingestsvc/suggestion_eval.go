// internal/services/ingestsvc/suggestion_eval.go
package ingestsvc

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// findCandidateSuggestions returns IDs of suggestions whose matchRule passes against rawBody.
func (s *IngestService) findCandidateSuggestions(sourceFamily string, rawBody map[string]any) []string {
	if s.suggestionSvc == nil {
		return nil
	}
	suggestions := s.suggestionSvc.GetByFamily(sourceFamily)
	var ids []string
	for _, sg := range suggestions {
		if evalSuggestionMatchRule(sg.MatchRule, rawBody) {
			ids = append(ids, sg.ID)
		}
	}
	return ids
}

// evalSuggestionMatchRule evaluates a SuggestionMatchRule against rawBody.
// type "all": every rule must pass. type "any": at least one must pass.
func evalSuggestionMatchRule(rule ingestmod.SuggestionMatchRule, rawBody map[string]any) bool {
	if len(rule.Rules) == 0 {
		return true
	}
	switch strings.ToLower(rule.Type) {
	case "any":
		for _, r := range rule.Rules {
			if evalSuggestionRule(r, rawBody) {
				return true
			}
		}
		return false
	default: // "all"
		for _, r := range rule.Rules {
			if !evalSuggestionRule(r, rawBody) {
				return false
			}
		}
		return true
	}
}

// evalSuggestionRule evaluates a single SuggestionRuleItem against rawBody.
func evalSuggestionRule(r ingestmod.SuggestionRuleItem, rawBody map[string]any) bool {
	val, found := getNestedValue(rawBody, r.Field)

	switch strings.ToLower(r.Operator) {
	case "exists":
		wantExists, _ := r.Value.(bool)
		return found == wantExists
	case "eq":
		if !found {
			return false
		}
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", r.Value)
	case "contains":
		if !found {
			return false
		}
		return strings.Contains(fmt.Sprintf("%v", val), fmt.Sprintf("%v", r.Value))
	default:
		return false
	}
}
