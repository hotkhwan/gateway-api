// internal/services/aimappingsvc/schema.go
package aimappingsvc

import "github.com/hotkhwan/gateway-api/models/ingestmod"

// AISuggestInput is the input to AIMappingService.Suggest.
type AISuggestInput struct {
	OrgID            string
	WorkspaceID      string
	UserID           string
	SourceFamily     string
	SamplePayload    map[string]any
	ExistingMappings []ingestmod.FieldMapping
}

// AISuggestResult is returned to the controller.
type AISuggestResult struct {
	Mode               string `json:"mode"`
	// mode: "aiAugmented" | "systemOnly" | "aiFailedFallback"
	Provider           string                `json:"provider"`
	Model              string                `json:"model"`
	PromptVersion      string                `json:"promptVersion"`
	SchemaVersion      string                `json:"schemaVersion"`
	SuggestedEventType string                `json:"suggestedEventType"`
	FieldMappings      []SuggestionFieldMap  `json:"fieldMappings"`
	MatchRules         []MatchRuleSuggestion `json:"matchRules"`
	Conflicts          []SuggestConflict     `json:"conflicts,omitempty"`
	Warnings           []string              `json:"warnings,omitempty"`
	Diagnostics        SuggestDiagnostics    `json:"diagnostics"`
	Confidence         float64               `json:"confidence"`
}

// SuggestionFieldMap describes a field mapping suggested to the user.
type SuggestionFieldMap struct {
	SourceField     string            `json:"sourceField"`
	TargetField     string            `json:"targetField"`
	ValueCodes      map[string]string `json:"valueCodes,omitempty"`
	ValueCodeSource string            `json:"valueCodeSource,omitempty"` // "system" | "aiInferred"
	Source          string            `json:"source"`                    // "system" | "ai" | "merged"
}

// MatchRuleSuggestion describes a single condition used to identify this payload type.
type MatchRuleSuggestion struct {
	FieldPath string `json:"fieldPath"`
	Operator  string `json:"operator"`
	Value     any    `json:"value,omitempty"`
	Required  bool   `json:"required"`
	Source    string `json:"source"` // "system" | "ai" | "merged"
	Reason    string `json:"reason,omitempty"`
}

// SuggestConflict records a field where system and AI suggestions disagreed.
type SuggestConflict struct {
	FieldPath   string `json:"fieldPath"`
	SystemValue any    `json:"systemValue"`
	AIValue     any    `json:"aiValue"`
	Resolution  string `json:"resolution"` // "systemKept" | "aiApplied" | "needsReview"
}

// SuggestDiagnostics carries internal metrics about the suggestion pipeline run.
type SuggestDiagnostics struct {
	SystemSuggestionUsed  bool  `json:"systemSuggestionUsed"`
	AIParseSuccess        bool  `json:"aiParseSuccess"`
	AIValidationSuccess   bool  `json:"aiValidationSuccess"`
	ObservedPathsCount    int   `json:"observedPathsCount"`
	AIOutputFieldsCount   int   `json:"aiOutputFieldsCount"`
	PayloadReducedBytes   int   `json:"payloadReducedBytes"`
	AILatencyMs           int64 `json:"aiLatencyMs"`
	EnumContextFieldsUsed int   `json:"enumContextFieldsUsed"`
}

// ValidationError describes a single validation failure in AI output.
type ValidationError struct {
	Kind    string // "schemaError" | "unknownOperator" | "unknownPath" | "weakMatchRule"
	Field   string
	Message string
}

// MergePolicy defines which paths are locked by the system and which can be extended by AI.
type MergePolicy struct {
	SystemLockedPaths []string
	AIExtendablePaths []string
}

// UpsertConfigRequest carries fields to update in a WorkspaceAIConfig.
type UpsertConfigRequest struct {
	Enabled          bool   `json:"enabled"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	ApiKey           string `json:"apiKey,omitempty"`           // plaintext — encrypted before storage
	DefaultTimeoutMs int    `json:"defaultTimeoutMs,omitempty"`
	MaxInputBytes    int    `json:"maxInputBytes,omitempty"`
}
