// models/ingestmod/sourceProfile.go
package ingestmod

import "time"

// SourceProfile defines source family behavior and mode.
// Collection: source_profiles
// Mode values: "active" | "comingSoon" | "mock"
type SourceProfile struct {
	SourceFamily string    `json:"sourceFamily" bson:"sourceFamily"`
	DisplayName  string    `json:"displayName"  bson:"displayName"`
	Mode         string    `json:"mode"         bson:"mode"` // active | comingSoon | mock
	CreatedAt    time.Time `json:"createdAt"    bson:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"    bson:"updatedAt"`
}
