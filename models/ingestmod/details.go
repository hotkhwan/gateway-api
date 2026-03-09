// models/ingestmod/details.go
package ingestmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EventDetail represents approved/normalized events stored in event_details.
// BSON tags match the NormalizedEvent shape written by the normalizer consumer.
// JSON tags provide a flat, FE-friendly projection.
type EventDetail struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	EventId  string             `bson:"eventId"        json:"eventId"`
	TenantId string             `bson:"tenantId"       json:"tenantId"`

	// Event classification
	EventType     string `bson:"eventType"                json:"eventType"`
	EventClass    string `bson:"eventClass,omitempty"      json:"eventClass,omitempty"`
	EventSeverity string `bson:"eventSeverity,omitempty"   json:"eventSeverity,omitempty"`

	// Nested structures — match NormalizedEvent BSON shape
	Source   SourceInfo   `bson:"source"   json:"source"`
	Location LocationInfo `bson:"location" json:"location"`

	// Geo enrichment
	Geo         GeoInfo         `bson:"geo"         json:"geo"`
	GeoCell     GeoCellInfo     `bson:"geoCell"     json:"geoCell"`
	ByAdminArea ByAdminAreaInfo `bson:"byAdminArea" json:"byAdminArea"`

	// Payload and binaries
	Payload    map[string]any `bson:"payload"    json:"payload"`
	BinaryRefs []BinaryRef    `bson:"binaryRefs" json:"binaryRefs"`

	// Normalization metadata
	Meta NormalizationMeta `bson:"meta" json:"meta"`

	// Timing
	OccurredAt time.Time `bson:"occurredAt"          json:"occurredAt"`
	CreatedAt  time.Time `bson:"createdAt"            json:"createdAt"`
	UpdatedAt  time.Time `bson:"updatedAt"            json:"updatedAt"`

	// Top-level convenience fields (FE backward compat)
	// Projected from nested structures for list/detail API
	OrgId string  `bson:"-" json:"orgId"`
	Name  string  `bson:"-" json:"name"`
	Lat   float64 `bson:"-" json:"lat"`
	Lng   float64 `bson:"-" json:"lng"`
}

// ProjectFlatFields populates top-level convenience fields from nested structures.
func (e *EventDetail) ProjectFlatFields() {
	e.OrgId = e.Source.OrgId
	e.Name = e.Source.DeviceName
	e.Lat = e.Location.Lat
	e.Lng = e.Location.Lng
}
