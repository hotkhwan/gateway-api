// internal/services/aiconfigdraftsvc/schema.go
package aiconfigdraftsvc

import (
	"time"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// ConfigDraft is a pending configuration built from a natural-language prompt.
// Stored in MongoDB collection: ai_config_drafts
type ConfigDraft struct {
	DraftID         string                      `json:"draftId"         bson:"draftId"`
	WorkspaceID     string                      `json:"workspaceId"     bson:"workspaceId"`
	Status          string                      `json:"status"          bson:"status"` // "incomplete"|"ready"|"reviewed"|"published"|"deployed"
	SourceFamily    string                      `json:"sourceFamily"    bson:"sourceFamily"`
	MatchConditions []ingestmod.SuggestionRuleItem `json:"matchConditions" bson:"matchConditions"`
	MissingFields   []MissingFieldHint          `json:"missingFields"   bson:"missingFields"`
	Warnings        []string                    `json:"warnings"        bson:"warnings"`
	ReviewSummary   []string                    `json:"reviewSummary"   bson:"reviewSummary"`
	RedactedPrompt  string                      `json:"redactedPrompt"  bson:"redactedPrompt"`
	CreatedBy       string                      `json:"createdBy"       bson:"createdBy"`
	CreatedAt       time.Time                   `json:"createdAt"       bson:"createdAt"`
	UpdatedAt       time.Time                   `json:"updatedAt"       bson:"updatedAt"`
}

// MissingFieldHint describes a required field that is not yet resolved in the draft.
type MissingFieldHint struct {
	Field     string `json:"field"     bson:"field"`
	Reason    string `json:"reason"    bson:"reason"`
	ForAction string `json:"forAction" bson:"forAction"`
}

// DraftIntent holds parsed intent extracted from a natural-language prompt.
type DraftIntent struct {
	SourceFamily    string
	Conditions      []IntentCondition
	DeliveryActions []IntentAction
	RawPrompt       string
	ParsedAt        time.Time
}

// IntentCondition represents a single parsed condition from the user's prompt.
type IntentCondition struct {
	RawPhrase     string
	FieldHint     string
	Operator      string
	ValueHint     string
	Resolved      bool
	ResolvedPath  string
	ResolvedValue any
}

// IntentAction represents a single delivery target parsed from the user's prompt.
type IntentAction struct {
	Type      string // "webhook" | "line" | "discord" | "mqtt"
	RawTarget string // masked
	SecretRef string
	Resolved  bool
}

// DryRunResult holds the outcome of simulating a draft against a sample payload.
type DryRunResult struct {
	Matched             bool     `json:"matched"`
	WebhookTargetsCount int      `json:"webhookTargetsCount"`
	LineTargetsCount    int      `json:"lineTargetsCount"`
	DiscordTargetsCount int      `json:"discordTargetsCount"`
	IncompleteTargets   []string `json:"incompleteTargets"`
	EvaluationDetails   []string `json:"evaluationDetails"`
}

// RefineRequest carries answers to missing field hints.
type RefineRequest struct {
	Answers map[string]string `json:"answers"` // field name → value
}
