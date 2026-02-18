// models/authzmod/orgUnit.go
package authzmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type OrgUnit struct {
	ID     primitive.ObjectID `bson:"_id,omitempty"` // internal
	UnitId string             `bson:"unitId"`        // external stable id

	TenantId string `bson:"tenantId"`
	OrgId    string `bson:"orgId"` // FK to Organization.OrgId

	ParentUnitId *string `bson:"parentUnitId,omitempty"` // FK to OrgUnit.UnitId

	Name        string `bson:"name"`
	Description string `bson:"description,omitempty"`
	IsRoot      bool   `bson:"isRoot"`

	CreatedBy string    `bson:"createdBy"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedBy string    `bson:"updatedBy"`
	UpdatedAt time.Time `bson:"updatedAt"`
}
