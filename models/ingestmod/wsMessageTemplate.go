// models/ingestmod/wsMessageTemplate.go
package ingestmod

import "time"

// WorkspaceMessageTemplate is a standalone workspace-scoped notification template.
// Used by TemplateDeliveryBinding.MessageTemplateID to render channel messages.
// Supports Go text/template syntax in Body.
// MongoDB collection: workspace_message_templates
type WorkspaceMessageTemplate struct {
	ID          string    `bson:"_id"         json:"id"`
	WorkspaceID string    `bson:"workspaceId" json:"workspaceId"`
	Name        string    `bson:"name"        json:"name"`
	Channel     string    `bson:"channel"     json:"channel"` // line|webhook|telegram|discord
	Body        string    `bson:"body"        json:"body"`    // Go text/template string
	Locale      string    `bson:"locale"      json:"locale"`  // e.g. "th", "en"
	CreatedAt   time.Time `bson:"createdAt"   json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}
