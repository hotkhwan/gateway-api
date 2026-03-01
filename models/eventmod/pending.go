package eventmod

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DeviceIdentity represents normalized device reference
type DeviceIdentity struct {
	Type string `json:"type" json:"type"`   // "camera", "sensor", "face", "device"
	ID   string `json:"id" json:"id"`     // device identifier
}

// EventManagement represents pending events awaiting approval
type EventManagement struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	EventId      string             `bson:"eventId" json:"eventId"`
	TenantId     string             `bson:"tenantId" json:"tenantId"`
	OrgId        string             `bson:"orgId" json:"orgId"`

	// Editable metadata (by admin)
	Name      string  `bson:"name" json:"name"`
	Lat       float64 `bson:"lat" json:"lat"`
	Lng       float64 `bson:"lng" json:"lng"`
	EventType string  `bson:"eventType" json:"eventType"` // LPR_Brand, FACE_Brand, camera_Brand, IOT_Brand, etc.

	// Approval status
	Status     bool   `bson:"status" json:"status"`           // false = pending, true = approved
	StatusName string `bson:"statusName" json:"statusName"` // "pending", "approved", "rejected"

	// Device Identity Normalization
	DeviceRef     *DeviceIdentity   `bson:"deviceRef,omitempty" json:"deviceRef,omitempty"`     // { type: "camera"| "sensor"| "face"| "device", id: "..." }
	DeviceKey     string           `bson:"deviceKey,omitempty" json:"deviceKey,omitempty"`     // "camera:cam-001", "device:dev-001" (canonical key for locking)
	RawAliases    json.RawMessage `bson:"rawAliases,omitempty" json:"rawAliases,omitempty"` // Original raw field mapping

	// Raw event data
	RawBody     json.RawMessage `bson:"rawBody" json:"rawBody"`
	ContentType string          `bson:"contentType" json:"contentType"`
	SourceIp    string          `bson:"sourceIp" json:"sourceIp"`

	// Auto-detection suggestion
	SuggestedType string `bson:"suggestedType,omitempty" json:"suggestedType,omitempty"`

	// Timestamps
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`

	// Admin action
	ApprovedBy string    `bson:"approvedBy,omitempty" json:"approvedBy,omitempty"`
	ApprovedAt time.Time `bson:"approvedAt,omitempty" json:"approvedAt,omitempty"`
}
