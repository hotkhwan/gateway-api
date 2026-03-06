// models/ingestmod/normalization.go
package ingestmod

import "time"

// CanonicalEvent — normalized event published to Kafka raw.events after approval.
// Produced by the approval gate; consumed by the normalizer service.
type CanonicalEvent struct {
	EventId    string         `json:"eventId"    bson:"eventId"`
	EventType  string         `json:"eventType"  bson:"eventType"`
	OccurredAt time.Time      `json:"occurredAt" bson:"occurredAt"`
	Source     SourceInfo     `json:"source"     bson:"source"`
	Location   LocationInfo   `json:"location"   bson:"location"`
	Payload    map[string]any `json:"payload"    bson:"payload"`
	CreatedAt  time.Time      `json:"createdAt"  bson:"createdAt"`
}

// SourceInfo identifies the originating device.
type SourceInfo struct {
	DeviceId   string `json:"deviceId"   bson:"deviceId"`
	DeviceType string `json:"deviceType" bson:"deviceType"`
	Vendor     string `json:"vendor"     bson:"vendor"`
	OrgId      string `json:"orgId"      bson:"orgId"`
}

// LocationInfo holds the resolved geographic coordinates.
// Values are derived from fieldMappings targeting location.lat / location.lng.
type LocationInfo struct {
	Lat float64 `json:"lat" bson:"lat"`
	Lng float64 `json:"lng" bson:"lng"`
}
