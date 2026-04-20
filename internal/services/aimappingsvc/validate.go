// internal/services/aimappingsvc/validate.go
package aimappingsvc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/gateways/aiprovider"
)

var (
	allowedOperators    = map[string]bool{"eq": true, "exists": true, "contains": true}
	upperSnakeCaseRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// ValidateAIOutput validates the parsed AI output.
// Returns a list of validation errors (empty = valid).
func ValidateAIOutput(raw aiprovider.AISuggestRawResult, observedPaths []string) []ValidationError {
	var errs []ValidationError

	// Build a set of known paths for fast lookup.
	knownPaths := make(map[string]bool, len(observedPaths))
	for _, p := range observedPaths {
		knownPaths[p] = true
	}

	// Validate suggestedEventType — must be UPPER_SNAKE_CASE.
	if raw.SuggestedEventType == "" {
		errs = append(errs, ValidationError{
			Kind:    "schemaError",
			Field:   "suggestedEventType",
			Message: "suggestedEventType is required",
		})
	} else {
		normalized := strings.ToUpper(strings.ReplaceAll(raw.SuggestedEventType, " ", "_"))
		if !upperSnakeCaseRegex.MatchString(normalized) {
			errs = append(errs, ValidationError{
				Kind:    "schemaError",
				Field:   "suggestedEventType",
				Message: fmt.Sprintf("suggestedEventType %q is not valid UPPER_SNAKE_CASE", raw.SuggestedEventType),
			})
		}
	}

	// Validate field mappings.
	for i, fm := range raw.FieldMappings {
		field := fmt.Sprintf("fieldMappings[%d]", i)
		if fm.SourceField == "" {
			errs = append(errs, ValidationError{
				Kind:    "schemaError",
				Field:   field + ".sourceField",
				Message: "sourceField is required",
			})
		} else if !knownPaths[fm.SourceField] && !isKnownPathPrefix(fm.SourceField, observedPaths) {
			errs = append(errs, ValidationError{
				Kind:    "unknownPath",
				Field:   field + ".sourceField",
				Message: fmt.Sprintf("sourceField %q not in observed paths", fm.SourceField),
			})
		}
		if fm.TargetField == "" {
			errs = append(errs, ValidationError{
				Kind:    "schemaError",
				Field:   field + ".targetField",
				Message: "targetField is required",
			})
		}
		// Validate valueCodes keys are strings (they always are in Go's map[string]string, but
		// we validate the map itself is not nil when it would make no sense).
		_ = fm.ValueCodes // already typed as map[string]string — always valid
	}

	// Validate match rules.
	existsOnlyRuleCount := 0
	for i, rule := range raw.MatchRules {
		field := fmt.Sprintf("matchRules[%d]", i)

		// Operator must be in allowlist.
		if !allowedOperators[rule.Operator] {
			errs = append(errs, ValidationError{
				Kind:    "unknownOperator",
				Field:   field + ".operator",
				Message: fmt.Sprintf("operator %q is not allowed; must be one of: eq, exists, contains", rule.Operator),
			})
		}

		// FieldPath should be in observed paths.
		if rule.FieldPath == "" {
			errs = append(errs, ValidationError{
				Kind:    "schemaError",
				Field:   field + ".fieldPath",
				Message: "fieldPath is required",
			})
		} else if !knownPaths[rule.FieldPath] && !isKnownPathPrefix(rule.FieldPath, observedPaths) {
			errs = append(errs, ValidationError{
				Kind:    "unknownPath",
				Field:   field + ".fieldPath",
				Message: fmt.Sprintf("fieldPath %q not in observed paths", rule.FieldPath),
			})
		}

		// Count pure "exists" rules with no value.
		if rule.Operator == "exists" && rule.Value == nil {
			existsOnlyRuleCount++
		}
	}

	// Weak match rule check: if the only rules are "exists" without other discriminators.
	if len(raw.MatchRules) > 0 && existsOnlyRuleCount == len(raw.MatchRules) {
		errs = append(errs, ValidationError{
			Kind:    "weakMatchRule",
			Field:   "matchRules",
			Message: "all match rules are 'exists' without specific values — insufficient discriminators",
		})
	}

	return errs
}

// NormalizeSuggestedEventType normalizes an event type string to UPPER_SNAKE_CASE.
func NormalizeSuggestedEventType(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, " ", "_"))
}

// isKnownPathPrefix returns true if the candidate is a prefix segment of any observed path.
// This allows AI to reference parent paths that implicitly exist.
func isKnownPathPrefix(candidate string, observedPaths []string) bool {
	for _, p := range observedPaths {
		if strings.HasPrefix(p, candidate+".") || strings.HasPrefix(p, candidate+"[") {
			return true
		}
	}
	return false
}
