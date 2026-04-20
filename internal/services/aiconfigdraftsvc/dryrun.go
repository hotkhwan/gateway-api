// internal/services/aiconfigdraftsvc/dryrun.go
package aiconfigdraftsvc

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// DryRun simulates a ConfigDraft against a sample payload and returns a DryRunResult.
// It evaluates each match condition against the payload and tallies delivery targets.
func DryRun(draft *ConfigDraft, samplePayload map[string]any) DryRunResult {
	result := DryRunResult{
		IncompleteTargets: []string{},
		EvaluationDetails: []string{},
	}

	if draft == nil || samplePayload == nil {
		result.EvaluationDetails = append(result.EvaluationDetails, "draft or samplePayload is nil")
		return result
	}

	// Evaluate match conditions.
	allMatched := true
	for _, cond := range draft.MatchConditions {
		matched, detail := evaluateCondition(cond, samplePayload)
		result.EvaluationDetails = append(result.EvaluationDetails, detail)
		if !matched {
			allMatched = false
		}
	}

	if len(draft.MatchConditions) == 0 {
		allMatched = false
		result.EvaluationDetails = append(result.EvaluationDetails, "no match conditions defined")
	}

	result.Matched = allMatched

	// Count delivery targets by checking missing fields.
	for _, mf := range draft.MissingFields {
		result.IncompleteTargets = append(result.IncompleteTargets, fmt.Sprintf("%s (%s)", mf.ForAction, mf.Field))
	}

	// Tally action types from warnings/review summary as a proxy for action types.
	// In production this would be driven by a proper actions list on the draft.
	for _, w := range draft.Warnings {
		lower := strings.ToLower(w)
		switch {
		case strings.Contains(lower, "webhook"):
			result.WebhookTargetsCount++
		case strings.Contains(lower, "line"):
			result.LineTargetsCount++
		case strings.Contains(lower, "discord"):
			result.DiscordTargetsCount++
		}
	}

	return result
}

// evaluateCondition checks a single SuggestionRuleItem against a flat payload map.
func evaluateCondition(cond ingestmod.SuggestionRuleItem, payload map[string]any) (bool, string) {
	val, ok := getNestedValue(payload, cond.Field)
	if !ok {
		return false, fmt.Sprintf("field %q not found in payload", cond.Field)
	}

	switch strings.ToLower(cond.Operator) {
	case "eq", "==", "equals":
		matched := fmt.Sprintf("%v", val) == fmt.Sprintf("%v", cond.Value)
		if matched {
			return true, fmt.Sprintf("field %q == %v: MATCH", cond.Field, cond.Value)
		}
		return false, fmt.Sprintf("field %q value %v != %v: NO MATCH", cond.Field, val, cond.Value)

	case "exists":
		return true, fmt.Sprintf("field %q exists: MATCH", cond.Field)

	case "contains":
		strVal := fmt.Sprintf("%v", val)
		strCond := fmt.Sprintf("%v", cond.Value)
		if strings.Contains(strVal, strCond) {
			return true, fmt.Sprintf("field %q contains %q: MATCH", cond.Field, strCond)
		}
		return false, fmt.Sprintf("field %q value %q does not contain %q: NO MATCH", cond.Field, strVal, strCond)

	default:
		return false, fmt.Sprintf("unknown operator %q for field %q", cond.Operator, cond.Field)
	}
}

// getNestedValue resolves a dot-separated field path in the payload.
func getNestedValue(payload map[string]any, path string) (any, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := payload[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}
	return getNestedValue(nested, parts[1])
}
