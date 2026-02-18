// models/authzmod/org.go
package authzmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Organization struct {
	ID    primitive.ObjectID `bson:"_id,omitempty"` // internal
	OrgId string             `bson:"orgId"`         // external stable id

	TenantId    string `bson:"tenantId"`
	Name        string `bson:"name"`
	Description string `bson:"description"`

	CreatedBy string    `bson:"createdBy"`
	CreatedAt time.Time `bson:"createdAt"` // ✅ BSON Date
	UpdatedBy string    `bson:"updatedBy"`
	UpdatedAt time.Time `bson:"updatedAt"` // ✅ BSON Date

	SyncStatus string `bson:"syncStatus"`
}
