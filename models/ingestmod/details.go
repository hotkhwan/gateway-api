package ingestmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EventDetail represents approved/normalized events
type EventDetail struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	EventId  string             `bson:"eventId" json:"eventId"`
	TenantId string             `bson:"tenantId" json:"tenantId"`
	OrgId    string             `bson:"orgId" json:"orgId"`

	// Normalized metadata
	Name      string  `bson:"name" json:"name"`
	Lat       float64 `bson:"lat" json:"lat"`
	Lng       float64 `bson:"lng" json:"lng"`
	EventType string  `bson:"eventType" json:"eventType"`

	// Normalized data (based on event type template)
	NormalizedData map[string]any `bson:"normalizedData" json:"normalizedData"`

	// Source tracking
	SourceIp   string    `bson:"sourceIp" json:"sourceIp"`
	IngestedAt time.Time `bson:"ingestedAt" json:"ingestedAt"`
	ApprovedAt time.Time `bson:"approvedAt" json:"approvedAt"`

	// Timestamps
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`

	// Reference to pending event
	PendingEventId primitive.ObjectID `bson:"pendingEventId,omitempty" json:"pendingEventId,omitempty"`
}
