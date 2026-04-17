// models/authzmod/permissionProfile.go
package authzmod

import "time"

type PermissionProfile struct {
	ProfileID        string    `bson:"profileId"        json:"id"`
	TenantID         string    `bson:"tenantId"         json:"-"`
	WorkspaceID      string    `bson:"workspaceId"      json:"workspaceId"`
	Name             string    `bson:"name"             json:"name"`
	Description      string    `bson:"description"      json:"description,omitempty"`
	Status           bool      `bson:"status"           json:"status"`
	Relations        []string  `bson:"relations"        json:"relations"`
	OrgUnitIDs       []string  `bson:"orgUnitIds"       json:"orgUnitIds"`
	ResourceGroupIDs []string  `bson:"resourceGroupIds" json:"resourceGroupIds"`
	CreatedBy        string    `bson:"createdBy"        json:"createdBy"`
	CreatedAt        time.Time `bson:"createdAt"        json:"createdAt"`
	UpdatedAt        time.Time `bson:"updatedAt"        json:"updatedAt"`
}
