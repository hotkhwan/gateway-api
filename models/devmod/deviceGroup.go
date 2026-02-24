// models/devmod/deviceGroup.go
package devmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ResourceGroup — Mongo document
type ResourceGroup struct {
	InternalID    primitive.ObjectID `bson:"_id,omitempty"   json:"-"`
	GroupID       string             `bson:"groupId"         json:"id"`
	TenantID      string             `bson:"tenantId"        json:"tenantId"`
	OrgID         string             `bson:"orgId"           json:"orgId"`
	Name          string             `bson:"name"            json:"name"`
	Description   string             `bson:"description"     json:"description,omitempty"`
	ResourceType  string             `bson:"resourceType"    json:"resourceType,omitempty"` // "camera" | "sensor" | "" = all
	MapVisibility string             `bson:"mapVisibility"   json:"mapVisibility,omitempty"` // "public" | "private"
	CreatedBy     string             `bson:"createdBy"       json:"createdBy,omitempty"`
	SyncStatus    string             `bson:"syncStatus"      json:"syncStatus,omitempty"` // pending | synced | failed
	CreatedAt     time.Time          `bson:"createdAt"       json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt"       json:"updatedAt,omitempty"`
}
