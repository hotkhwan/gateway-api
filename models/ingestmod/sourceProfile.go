// models/ingestmod/sourceProfile.go
package ingestmod

import "time"

// SourceProfile defines source family behavior, reference extraction hints, and suggested match fields.
// Collection: source_profiles
type SourceProfile struct {
	SourceFamily         string    `json:"sourceFamily"       bson:"sourceFamily"`
	DisplayName          string    `json:"displayName"        bson:"displayName"`
	MultiRef             bool      `json:"multiRef"           bson:"multiRef"`
	RefRules             RefRules  `json:"refRules"           bson:"refRules"`
	SuggestedMatchFields []string  `json:"suggestedMatchFields" bson:"suggestedMatchFields"`
	CreatedAt            time.Time `json:"createdAt"          bson:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"          bson:"updatedAt"`
}

// RefRules defines how to extract references from raw payload.
type RefRules struct {
	PrimaryRefFields   []string `json:"primaryRefFields,omitempty"   bson:"primaryRefFields,omitempty"`
	SecondaryRefFields []string `json:"secondaryRefFields,omitempty" bson:"secondaryRefFields,omitempty"`
	SiteFields         []string `json:"siteFields,omitempty"         bson:"siteFields,omitempty"`
}
