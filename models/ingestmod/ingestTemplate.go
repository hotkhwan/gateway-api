// models/ingestmod/ingestTemplate.go
package ingestmod

import "time"

// IngestTemplate is the classifier layer in the 3-layer delivery model.
// It defines how to identify and map raw event payloads from a specific sourceFamily.
// Separate from the legacy MappingTemplate (which embeds delivery targets).
// MongoDB collection: ingest_templates
type IngestTemplate struct {
	ID           string         `bson:"_id"          json:"id"`
	WorkspaceID  string         `bson:"workspaceId"  json:"workspaceId"`
	Name         string         `bson:"name"         json:"name"`
	SourceFamily string         `bson:"sourceFamily" json:"sourceFamily"`
	MatchRules   []MatchRule    `bson:"matchRules"   json:"matchRules"`   // fingerprint match criteria
	FieldMapping map[string]any `bson:"fieldMapping" json:"fieldMapping"` // raw→canonical field mapping
	Enabled      bool           `bson:"enabled"      json:"enabled"`
	CreatedAt    time.Time      `bson:"createdAt"    json:"createdAt"`
	UpdatedAt    time.Time      `bson:"updatedAt"    json:"updatedAt"`
}
