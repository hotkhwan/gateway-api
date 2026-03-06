// models/ingestmod/fieldmapping.go
package ingestmod

import "time"

// FieldMapping maps a source path in rawBody to a canonical target path.
// Stored as a snapshot on PendingEvent — changing a template does not affect approved events.
type FieldMapping struct {
	TargetPath string    `json:"targetPath" bson:"targetPath"`
	SourcePath string    `json:"sourcePath" bson:"sourcePath"`
	Transform  string    `json:"transform"  bson:"transform"`
	Required   bool      `json:"required"   bson:"required"`
	Confidence float64   `json:"confidence" bson:"confidence"`
	UpdatedAt  time.Time `json:"updatedAt"  bson:"updatedAt"`
}
