// models/ingestmod/rejectedPayloadPattern.go
package ingestmod

import "time"

// RejectedPayloadPattern records a sourceFamily+fingerprint pair that an operator
// has explicitly rejected. Future payloads matching this pair are dropped silently.
type RejectedPayloadPattern struct {
	PatternId    string    `json:"patternId"    bson:"patternId"`
	OrgId        string    `json:"orgId"        bson:"orgId"`
	SourceFamily string    `json:"sourceFamily" bson:"sourceFamily"`
	Fingerprint  string    `json:"fingerprint"  bson:"fingerprint"`
	Reason       string    `json:"reason"       bson:"reason"`
	CreatedBy    string    `json:"createdBy"    bson:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"    bson:"createdAt"`
}
