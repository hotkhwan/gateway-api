// models/workspacemod/workspace.go
package workspacemod

import "time"

// Workspace is phibek's operational container — not the same as klynx Organization.
// Each klynx org gets exactly one phibek workspace upon org creation.
// MongoDB collection: workspaces
type Workspace struct {
	WorkspaceID  string    `bson:"workspaceId"            json:"workspaceId"`
	KlynxOrgID   string    `bson:"klynxOrgId"             json:"klynxOrgId"`   // ref back to klynx org
	TenantID     string    `bson:"tenantId"               json:"tenantId"`     // Keycloak realm
	OwnerUserID  string    `bson:"ownerUserId,omitempty"  json:"ownerUserId,omitempty"`
	Name         string    `bson:"name"                   json:"name"`
	Status       string    `bson:"status"                 json:"status"`       // active | suspended | archived
	EventURI     string    `bson:"eventUri"               json:"eventUri"`     // /events/{workspaceId}/
	CreatedAt    time.Time `bson:"createdAt"              json:"createdAt"`
	UpdatedAt    time.Time `bson:"updatedAt"              json:"updatedAt"`
}

// WorkspaceStatus values
const (
	WorkspaceActive   = "active"
	WorkspaceSuspended = "suspended"
	WorkspaceArchived  = "archived"
)
