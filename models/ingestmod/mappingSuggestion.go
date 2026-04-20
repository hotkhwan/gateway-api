// models/ingestmod/mappingSuggestion.go
package ingestmod

// MappingSuggestion is a system-owned preset that helps customers configure
// field mappings quickly. Read-only — shipped with the binary via embed.FS.
type MappingSuggestion struct {
	ID            string               `json:"id"`
	SourceFamily  string               `json:"sourceFamily"`
	DisplayName   string               `json:"displayName"`
	MatchRule     SuggestionMatchRule  `json:"matchRule"`
	FieldMappings []SuggestionFieldMap `json:"fieldMappings"`
	SamplePayload map[string]any       `json:"samplePayload"`
}

// SuggestionMatchRule describes conditions used to select this suggestion.
type SuggestionMatchRule struct {
	Type  string               `json:"type"` // "all" | "any"
	Rules []SuggestionRuleItem `json:"rules"`
}

// SuggestionRuleItem is a single condition in a SuggestionMatchRule.
type SuggestionRuleItem struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // "eq" | "exists" | "contains"
	Value    any    `json:"value"`
}

// SuggestionFieldMap maps a source payload field path to a target field path.
type SuggestionFieldMap struct {
	SourceField string            `json:"sourceField"`
	TargetField string            `json:"targetField"`
	// ValueCodes maps raw integer/string codes to human-readable labels.
	// Normalizer generates a `<targetField>_label` field for matched codes.
	// Example: {"0": "Unknown", "1": "Male", "2": "Female"}
	ValueCodes  map[string]string `json:"valueCodes,omitempty"`
}
