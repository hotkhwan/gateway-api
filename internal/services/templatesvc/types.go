// internal/services/templatesvc/types.go
package templatesvc

import "github.com/hotkhwan/gateway-api/models/ingestmod"

// CreateTemplateInput is the request body for POST /ingest/mappingTemplates.
type CreateTemplateInput struct {
	Name                string                             `json:"name"`
	Enabled             *bool                              `json:"enabled,omitempty"`
	SourceFamily        string                             `json:"sourceFamily,omitempty"`
	FinalEventType      string                             `json:"finalEventType,omitempty"`
	Priority            int                                `json:"priority,omitempty"`
	Match               ingestmod.MatchRule                `json:"match"`
	MatchAll            []ingestmod.MatchCondition         `json:"matchAll,omitempty"`
	MatchAny            []ingestmod.MatchCondition         `json:"matchAny,omitempty"`
	DeliveryMatchAll    []ingestmod.MatchCondition         `json:"deliveryMatchAll,omitempty"`
	DeliveryMatchAny    []ingestmod.MatchCondition         `json:"deliveryMatchAny,omitempty"`
	Mappings            []ingestmod.FieldMapping           `json:"mappings"`
	DLQ                 *ingestmod.DLQConfig               `json:"dlq,omitempty"`
	DefaultLocale       string                             `json:"defaultLocale,omitempty"`
	MessageTemplates    []ingestmod.MessageTemplate        `json:"messageTemplates,omitempty"`
	ClassificationRules []ingestmod.ClassificationRule     `json:"classificationRules,omitempty"`
	DeliveryTargets     []ingestmod.TemplateDeliveryTarget `json:"deliveryTargets,omitempty"`
}

// UpdateTemplateInput is the request body for PATCH /ingest/mappingTemplates/:templateId.
// Only non-nil / non-empty fields are applied.
//
// Slice fields (MatchAll, MatchAny, Mappings, MessageTemplates, ClassificationRules,
// DeliveryTargets) use nil vs. empty-slice semantics:
//   - nil (absent in request)  → field is left unchanged
//   - []   (explicit empty)    → field is cleared
type UpdateTemplateInput struct {
	Name                *string                            `json:"name,omitempty"`
	Enabled             *bool                              `json:"enabled,omitempty"`
	SourceFamily        *string                            `json:"sourceFamily,omitempty"`
	FinalEventType      *string                            `json:"finalEventType,omitempty"`
	Priority            *int                               `json:"priority,omitempty"`
	Match               *ingestmod.MatchRule               `json:"match,omitempty"`
	MatchAll            []ingestmod.MatchCondition         `json:"matchAll,omitempty"`
	MatchAny            []ingestmod.MatchCondition         `json:"matchAny,omitempty"`
	DeliveryMatchAll    []ingestmod.MatchCondition         `json:"deliveryMatchAll,omitempty"`
	DeliveryMatchAny    []ingestmod.MatchCondition         `json:"deliveryMatchAny,omitempty"`
	Mappings            []ingestmod.FieldMapping           `json:"mappings,omitempty"`
	DLQ                 *ingestmod.DLQConfig               `json:"dlq,omitempty"`
	DefaultLocale       *string                            `json:"defaultLocale,omitempty"`
	MessageTemplates    []ingestmod.MessageTemplate        `json:"messageTemplates,omitempty"`
	ClassificationRules []ingestmod.ClassificationRule     `json:"classificationRules,omitempty"`
	DeliveryTargets     []ingestmod.TemplateDeliveryTarget `json:"deliveryTargets,omitempty"`
}

// ListTemplatesInput carries query parameters for the list endpoint.
type ListTemplatesInput struct {
	OrgId     string
	Search    string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}
