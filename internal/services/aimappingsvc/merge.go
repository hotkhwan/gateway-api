// internal/services/aimappingsvc/merge.go
package aimappingsvc

import (
	"github.com/hotkhwan/gateway-api/internal/gateways/aiprovider"
)

// MergeWithPolicy merges system suggestion and AI output according to the 3-tier policy.
//
// Tier 1: systemLockedFields — AI cannot override. System value is always kept.
// Tier 2: aiExtendableFields — AI can add new paths not present in the system suggestion.
// Tier 3: conflicts — fields where both system and AI have a value that differs → needsReview.
func MergeWithPolicy(
	systemMappings []SuggestionFieldMap,
	systemRules []MatchRuleSuggestion,
	aiResult aiprovider.AISuggestRawResult,
	observedPaths []string,
) (mappings []SuggestionFieldMap, rules []MatchRuleSuggestion, conflicts []SuggestConflict) {
	// Build index of system mappings keyed by sourceField.
	systemMapIdx := make(map[string]SuggestionFieldMap, len(systemMappings))
	for _, sm := range systemMappings {
		systemMapIdx[sm.SourceField] = sm
	}

	// Build index of system rules keyed by fieldPath.
	systemRuleIdx := make(map[string]MatchRuleSuggestion, len(systemRules))
	for _, sr := range systemRules {
		systemRuleIdx[sr.FieldPath] = sr
	}

	// Build set of observed paths for bounds checking.
	observedSet := make(map[string]bool, len(observedPaths))
	for _, p := range observedPaths {
		observedSet[p] = true
	}

	// --- Merge field mappings ---
	// Start with all system mappings (tier 1: system locked).
	for _, sm := range systemMappings {
		mappings = append(mappings, sm)
	}

	// Iterate AI mappings and either extend or detect conflicts.
	for _, aiMap := range aiResult.FieldMappings {
		if sysFM, exists := systemMapIdx[aiMap.SourceField]; exists {
			// Both system and AI have an entry for this sourceField.
			if sysFM.TargetField != aiMap.TargetField {
				// Conflict: system is kept (tier 1), record conflict.
				conflicts = append(conflicts, SuggestConflict{
					FieldPath:   aiMap.SourceField,
					SystemValue: sysFM.TargetField,
					AIValue:     aiMap.TargetField,
					Resolution:  "systemKept",
				})
			}
			// System value already in output — skip AI entry.
			continue
		}

		// AI adds a new path not present in system (tier 2: ai extendable).
		valueCodeSource := ""
		if len(aiMap.ValueCodes) > 0 {
			valueCodeSource = "aiInferred"
		}
		mappings = append(mappings, SuggestionFieldMap{
			SourceField:     aiMap.SourceField,
			TargetField:     aiMap.TargetField,
			ValueCodes:      aiMap.ValueCodes,
			ValueCodeSource: valueCodeSource,
			Source:          "ai",
		})
	}

	// --- Merge match rules ---
	// Start with all system rules (tier 1: system locked).
	for _, sr := range systemRules {
		rules = append(rules, sr)
	}

	// Iterate AI rules and either extend or detect conflicts.
	for _, aiRule := range aiResult.MatchRules {
		if sysRule, exists := systemRuleIdx[aiRule.FieldPath]; exists {
			// Both have a rule for this fieldPath.
			if sysRule.Operator != aiRule.Operator {
				// Operator conflict: system kept.
				conflicts = append(conflicts, SuggestConflict{
					FieldPath:   aiRule.FieldPath,
					SystemValue: sysRule.Operator,
					AIValue:     aiRule.Operator,
					Resolution:  "systemKept",
				})
			}
			// System rule already in output — skip AI rule.
			continue
		}

		// AI adds a new rule not present in system (tier 2: ai extendable).
		rules = append(rules, MatchRuleSuggestion{
			FieldPath: aiRule.FieldPath,
			Operator:  aiRule.Operator,
			Value:     aiRule.Value,
			Required:  aiRule.Required,
			Source:    "ai",
			Reason:    aiRule.Reason,
		})
	}

	return mappings, rules, conflicts
}
