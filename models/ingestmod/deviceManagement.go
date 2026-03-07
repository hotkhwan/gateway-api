// models/ingestmod/deviceManagement.go
package ingestmod

import "time"

// DeviceManagement defines override/enrichment rules for a source entity.
// Collection: device_management
type DeviceManagement struct {
	DeviceMgmtId string  `json:"deviceMgmtId" bson:"deviceMgmtId"`
	TenantId     string  `json:"tenantId"      bson:"tenantId"`
	OrgId        string  `json:"orgId"         bson:"orgId"`
	SourceFamily string  `json:"sourceFamily"  bson:"sourceFamily"`
	EntityType   string  `json:"entityType"    bson:"entityType"`   // "channel", "device", "sourceSerial"
	EntityId     string  `json:"entityId"      bson:"entityId"`     // e.g. "31", "EDGEAI-8ch"
	DeviceId     string  `json:"deviceId,omitempty"   bson:"deviceId,omitempty"`
	Lat          float64 `json:"lat,omitempty"        bson:"lat,omitempty"`
	Lng          float64 `json:"lng,omitempty"        bson:"lng,omitempty"`
	Site         string  `json:"site,omitempty"       bson:"site,omitempty"`
	Zone         string  `json:"zone,omitempty"       bson:"zone,omitempty"`
	CreatedAt    time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt" bson:"updatedAt"`
}

// ResolvedRef represents one reference in a multi-ref resolution.
type ResolvedRef struct {
	Role       string `json:"role"       bson:"role"`       // "origin", "gateway", "serial"
	EntityType string `json:"entityType" bson:"entityType"` // "channel", "device", "sourceSerial"
	EntityId   string `json:"entityId"   bson:"entityId"`
}
