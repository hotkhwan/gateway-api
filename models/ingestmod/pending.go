// models/ingestmod/pending.go
package ingestmod

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)


// DeviceIdentity represents normalized device reference
type DeviceIdentity struct {
	Type string `json:"type"` // "camera", "sensor", "face", "device"
	ID   string `json:"id"`   // device identifier
}

// EventManagement represents pending events awaiting approval
type EventManagement struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	EventId  string             `bson:"eventId" json:"eventId"`
	TenantId string             `bson:"tenantId" json:"tenantId"`
	OrgId    string             `bson:"orgId" json:"orgId"`

	// Editable metadata (by admin)
	Name        string   `bson:"name" json:"name"`
	Description *string  `bson:"description,omitempty" json:"description,omitempty"`
	Lat         float64  `bson:"lat" json:"lat"`
	Lng         float64  `bson:"lng" json:"lng"`
	EventType   string   `bson:"eventType" json:"eventType"`                   // LPR_Brand, FACE_Brand, camera_Brand, IOT_Brand, etc.
	Priority    string   `bson:"priority,omitempty" json:"priority,omitempty"` // "low", "medium", "high"
	Tags        []string `bson:"tags,omitempty" json:"tags,omitempty"`         // e.g., ["LPR", "FACE", "IOT"]

	// Approval status
	Status     bool   `bson:"status" json:"status"`         // false = pending, true = approved
	StatusName string `bson:"statusName" json:"statusName"` // "pending", "approved", "rejected"

	// Device Identity Normalization
	DeviceRef  *DeviceIdentity `bson:"deviceRef,omitempty" json:"deviceRef,omitempty"`   // { type: "camera"| "sensor"| "face"| "device", id: "..." }
	DeviceKey  string          `bson:"deviceKey,omitempty" json:"deviceKey,omitempty"`   // "camera:cam-001", "device:dev-001" (canonical key for locking)
	RawAliases json.RawMessage `bson:"rawAliases,omitempty" json:"rawAliases,omitempty"` // Original raw field mapping

	// Raw event data — stored as BSON document (not binary)
	RawBody     map[string]any `bson:"rawBody" json:"rawBody"`
	ContentType string         `bson:"contentType" json:"contentType"`
	SourceIp    string         `bson:"sourceIp" json:"sourceIp"`

	// Fingerprint for template matching: vendor|protocol|deviceType|subType|eventType|schemaVersion|keyHash
	Fingerprint string `bson:"fingerprint,omitempty" json:"fingerprint,omitempty"`

	// Auto-detection suggestion
	SuggestedType string `bson:"suggestedType,omitempty" json:"suggestedType,omitempty"`

	// Timestamps
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`

	// Admin action
	ApprovedBy string    `bson:"approvedBy,omitempty" json:"approvedBy,omitempty"`
	ApprovedAt time.Time `bson:"approvedAt,omitempty" json:"approvedAt,omitempty"`
}

// PendingEvent — new canonical shape for event_management collection (PR2+)
// Replaces EventManagement once ingestrepo/ingestsvc are migrated (PR3)
//
// statusName values: "pending" | "mapped" | "approved" | "rejected"
type PendingEvent struct {
	EventId       string         `json:"eventId"               bson:"eventId"`
	OrgId         string         `json:"orgId"                 bson:"orgId"`
	EventType     string         `json:"eventType"             bson:"eventType"`
	RawBody       map[string]any `json:"rawBody"               bson:"rawBody"`
	FieldMappings []FieldMapping `json:"fieldMappings"         bson:"fieldMappings"`
	TemplateId    *string        `json:"templateId,omitempty"  bson:"templateId,omitempty"`
	Fingerprint   string         `json:"fingerprint,omitempty" bson:"fingerprint,omitempty"`
	StatusName    string         `json:"statusName"            bson:"statusName"`
	SourceIp      string         `json:"sourceIp,omitempty"    bson:"sourceIp,omitempty"`
	ContentType   string         `json:"contentType,omitempty" bson:"contentType,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"             bson:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"             bson:"updatedAt"`
}
