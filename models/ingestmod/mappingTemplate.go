// models/ingestmod/mappingTemplate.go
package ingestmod

import "time"

// MappingTemplate — collection: mapping_templates
// Defines how to map rawBody fields to canonical targets for a given device/event signature.
type MappingTemplate struct {
	TemplateId string         `json:"templateId" bson:"templateId"`
	OrgId      string         `json:"orgId"      bson:"orgId"`
	Name       string         `json:"name"       bson:"name"`
	Match      MatchRule      `json:"match"      bson:"match"`
	Mappings   []FieldMapping `json:"mappings"   bson:"mappings"`
	CreatedAt  time.Time      `json:"createdAt"  bson:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"  bson:"updatedAt"`
}

// MatchRule defines criteria used to auto-bind a template to an incoming event.
// Fingerprint matching uses: vendor|protocol|deviceType|deviceId|subType|eventType|rawSchemaVersion|rawBodyKeyHash
type MatchRule struct {
	Vendor           string `json:"vendor,omitempty"           bson:"vendor,omitempty"`
	Protocol         string `json:"protocol,omitempty"         bson:"protocol,omitempty"`
	DeviceType       string `json:"deviceType,omitempty"       bson:"deviceType,omitempty"`
	DeviceId         string `json:"deviceId,omitempty"         bson:"deviceId,omitempty"`
	SubType          string `json:"subType,omitempty"          bson:"subType,omitempty"`
	EventType        string `json:"eventType,omitempty"        bson:"eventType,omitempty"`
	RawSchemaVersion string `json:"rawSchemaVersion,omitempty" bson:"rawSchemaVersion,omitempty"`
	RawBodyKeyHash   string `json:"rawBodyKeyHash,omitempty"   bson:"rawBodyKeyHash,omitempty"`
}
