// models/ingestmod/binding.go
package ingestmod

import "time"

// DispatchStage values for TemplateDeliveryBinding.
const (
	DispatchStageNormalize = "normalize"
	DispatchStageRealtime  = "realtime"
)

// TemplateDeliveryBinding routes events from an IngestTemplate to a DeliveryTarget.
// Routing rules: dispatchStage + matchFields filter.
// 3-layer model: IngestTemplate (classifier) → TemplateDeliveryBinding (routing) → DeliveryTarget (destination).
// MongoDB collection: template_delivery_bindings
type TemplateDeliveryBinding struct {
	ID                string         `bson:"_id"                        json:"id"`
	WorkspaceID       string         `bson:"workspaceId"                json:"workspaceId"`
	TemplateID        string         `bson:"templateId"                 json:"templateId"`
	TargetID          string         `bson:"targetId"                   json:"targetId"`
	DispatchStage     string         `bson:"dispatchStage"              json:"dispatchStage"`              // "normalize" | "realtime"
	MatchFields       map[string]any `bson:"matchFields,omitempty"      json:"matchFields,omitempty"`      // event selector e.g. {"eventType":"intrusion"}
	MessageTemplateID string         `bson:"messageTemplateId,omitempty" json:"messageTemplateId,omitempty"` // ref to WorkspaceMessageTemplate
	Enabled           bool           `bson:"enabled"                    json:"enabled"`
	CreatedAt         time.Time      `bson:"createdAt"                  json:"createdAt"`
	UpdatedAt         time.Time      `bson:"updatedAt"                  json:"updatedAt"`
}
