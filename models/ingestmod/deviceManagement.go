// models/ingestmod/deviceManagement.go
package ingestmod

import "time"

// DeviceManagement defines override/enrichment rules for a source entity.
// Collection: device_management
type DeviceManagement struct {
	DeviceMgmtId     string     `json:"deviceMgmtId" bson:"deviceMgmtId"`
	TenantId         string     `json:"tenantId"      bson:"tenantId"`
	WorkspaceId      string     `json:"workspaceId"   bson:"workspaceId"`
	SourceFamily     string     `json:"sourceFamily"  bson:"sourceFamily"`
	EntityType       string     `json:"entityType"    bson:"entityType"` // "channel", "device", "sourceSerial"
	EntityId         string     `json:"entityId"      bson:"entityId"`   // e.g. "31", "EDGEAI-8ch"
	DeviceId         string     `json:"deviceId,omitempty"    bson:"deviceId,omitempty"`
	SerialNo         string     `json:"serialNo,omitempty"    bson:"serialNo,omitempty"` // vendor serial / edge unique ID (e.g. AIBOX "sn")
	Name             string     `json:"name,omitempty"        bson:"name,omitempty"`
	Description      string     `json:"description,omitempty" bson:"description,omitempty"`
	Lat              float64    `json:"lat,omitempty"         bson:"lat,omitempty"`
	Lng              float64    `json:"lng,omitempty"         bson:"lng,omitempty"`
	Site             string     `json:"site,omitempty"       bson:"site,omitempty"`
	Zone             string     `json:"zone,omitempty"       bson:"zone,omitempty"`
	LastOutboundHash string     `json:"lastOutboundHash,omitempty" bson:"lastOutboundHash,omitempty"` // sha256 of canonical accepted-fields payload of last klynx overlay PATCH per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.4; replay-only in v1 (no 409)
	DeregisteredAt   *time.Time `json:"deregisteredAt,omitempty" bson:"deregisteredAt,omitempty"`     // soft tombstone set by Deregister; cleared by AutoUpsertFromEvent on the next matching ingest event (revive). Never a hard delete. Per klynx-api/docs/contracts/device-camera-domain.md §5.10
	CreatedAt        time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt" bson:"updatedAt"`
}

// AutoUpsertHints carries candidate metadata extracted from an ingest event payload
// for auto-creating/merging device_management records.
type AutoUpsertHints struct {
	DeviceId    string
	SerialNo    string // vendor serial / edge unique ID (e.g. AIBOX "sn" field)
	Name        string
	Description string
	Lat         float64
	Lng         float64
	Site        string
	Zone        string
}

// ResolvedRef represents one reference in a multi-ref resolution.
type ResolvedRef struct {
	Role       string `json:"role"       bson:"role"`       // "origin", "gateway", "serial"
	EntityType string `json:"entityType" bson:"entityType"` // "channel", "device", "sourceSerial"
	EntityId   string `json:"entityId"   bson:"entityId"`
}
