// models/ingestmod/templateReview.go
package ingestmod

import "time"

// TemplateReview represents an unresolved payload sample awaiting template creation.
// Collection: template_reviews
type TemplateReview struct {
	ReviewId             string         `json:"reviewId"             bson:"reviewId"`
	TenantId             string         `json:"tenantId"             bson:"tenantId"`
	OrgId                string         `json:"orgId"                bson:"orgId"`
	SourceFamily         string         `json:"sourceFamily"         bson:"sourceFamily"`
	Fingerprint          string         `json:"fingerprint"          bson:"fingerprint"`
	SamplePayload        map[string]any `json:"samplePayload"        bson:"samplePayload"`
	SuggestedMatchFields []string       `json:"suggestedMatchFields" bson:"suggestedMatchFields"`
	Status               string         `json:"status"               bson:"status"` // "pending" | "archived"
	CreatedAt            time.Time      `json:"createdAt"            bson:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"            bson:"updatedAt"`
}
