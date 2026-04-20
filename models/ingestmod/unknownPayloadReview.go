// models/ingestmod/unknownPayloadReview.go
package ingestmod

import "time"

// UnknownPayloadReview represents a payload that could not be matched to any mapping template.
// Collection: unknown_payload_reviews
// Status values: "pending" | "rejected"
type UnknownPayloadReview struct {
	ReviewId               string         `json:"reviewId"               bson:"reviewId"`
	WorkspaceId                  string         `json:"workspaceId"              bson:"workspaceId"`
	SourceFamily           string         `json:"sourceFamily"           bson:"sourceFamily"`
	Fingerprint            string         `json:"fingerprint"            bson:"fingerprint"`
	SamplePayload          map[string]any `json:"samplePayload"          bson:"samplePayload"`
	CandidateSuggestionIds []string       `json:"candidateSuggestionIds" bson:"candidateSuggestionIds"`
	Status                 string         `json:"status"                 bson:"status"` // pending | rejected
	SeenCount              int            `json:"seenCount"              bson:"seenCount"`
	FirstSeenAt            time.Time      `json:"firstSeenAt"            bson:"firstSeenAt"`
	LastSeenAt             time.Time      `json:"lastSeenAt"             bson:"lastSeenAt"`
	CreatedAt              time.Time      `json:"createdAt"              bson:"createdAt"`
	UpdatedAt              time.Time      `json:"updatedAt"              bson:"updatedAt"`
}
