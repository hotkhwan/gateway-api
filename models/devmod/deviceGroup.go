// models/devmod/deviceGroup.go
package devmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ResourceGroup — Mongo document
type ResourceGroup struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	TenantID     string             `bson:"tenantId"       json:"tenantId"`
	OrgID        string             `bson:"orgId"          json:"orgId"`
	Name         string             `bson:"name"           json:"name"`
	Description  string             `bson:"description"    json:"description,omitempty"`
	ResourceType string             `bson:"resourceType"   json:"resourceType,omitempty"` // "camera" | "sensor" | "" = all types
	Public       bool               `bson:"public"         json:"public"`
	CreatedBy    string             `bson:"createdBy"      json:"createdBy,omitempty"`
	SyncStatus   string             `bson:"syncStatus"     json:"syncStatus,omitempty"` // pending | synced | failed
	CreatedAt    time.Time          `bson:"createdAt"      json:"createdAt"`
	UpdatedAt    time.Time          `bson:"updatedAt"      json:"updatedAt,omitempty"`
}
