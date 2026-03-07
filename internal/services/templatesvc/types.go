// internal/services/templatesvc/types.go
package templatesvc

import "github.com/hotkhwan/gateway-api/models/ingestmod"

// CreateTemplateInput is the request body for POST /ingest/mappingTemplates.
type CreateTemplateInput struct {
	Name                string                             `json:"name"`
	Match               ingestmod.MatchRule                `json:"match"`
	Mappings            []ingestmod.FieldMapping           `json:"mappings"`
	DLQ                 *ingestmod.DLQConfig               `json:"dlq,omitempty"`
	DefaultLocale       string                             `json:"defaultLocale,omitempty"`
	MessageTemplates    []ingestmod.MessageTemplate        `json:"messageTemplates,omitempty"`
	ClassificationRules []ingestmod.ClassificationRule     `json:"classificationRules,omitempty"`
	DeliveryTargets     []ingestmod.TemplateDeliveryTarget `json:"deliveryTargets,omitempty"`
}

// UpdateTemplateInput is the request body for PATCH /ingest/mappingTemplates/:templateId.
// Only non-nil / non-empty fields are applied.
type UpdateTemplateInput struct {
	Name                *string                            `json:"name,omitempty"`
	Match               *ingestmod.MatchRule               `json:"match,omitempty"`
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
