// internal/gateways/aiprovider/schema.go
package aiprovider

const (
	AISuggestPromptVersion  = "v1.0"
	AISuggestSchemaVersion  = "v1.0"
	ConfigDraftPromptVersion = "v1.0"
)

// AISuggestRawResult is the typed schema the AI must return.
// All providers must produce JSON that strictly matches this struct.
type AISuggestRawResult struct {
	SuggestedEventType string           `json:"suggestedEventType"`
	FieldMappings      []AIFieldMapping `json:"fieldMappings"`
	MatchRules         []AIMatchRule    `json:"matchRules"`
}

// AIFieldMapping describes how a source payload field maps to a target schema field,
// optionally including a value-code translation table.
type AIFieldMapping struct {
	SourceField string            `json:"sourceField"`
	TargetField string            `json:"targetField"`
	ValueCodes  map[string]string `json:"valueCodes,omitempty"`
}

// AIMatchRule describes a single field-level condition used to identify or filter events.
type AIMatchRule struct {
	FieldPath string `json:"fieldPath"`
	// Operator: "eq" | "exists" | "contains"
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	Required bool   `json:"required"`
	Reason   string `json:"reason,omitempty"`
}
