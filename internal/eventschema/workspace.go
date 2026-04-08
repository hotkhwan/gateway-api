// internal/eventschema/workspace.go
package eventschema

import "time"

// OrgCreatedEvent is published by klynx-api on klynx.org.created.v1
// when a new organization is created. phibek consumes this to provision a workspace.
type OrgCreatedEvent struct {
	OrgID     string    `json:"orgId"`
	TenantID  string    `json:"tenantId"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrgDeletedEvent is published by klynx-api on klynx.org.deleted.v1
// when an org is deleted or suspended. phibek suspends the linked workspace.
type OrgDeletedEvent struct {
	OrgID     string    `json:"orgId"`
	TenantID  string    `json:"tenantId"`
	DeletedBy string    `json:"deletedBy,omitempty"`
	DeletedAt time.Time `json:"deletedAt"`
}

// WorkspaceProvisionedEvent is published by phibek on phibek.workspace.provisioned.v1
// after a workspace is created. klynx-api consumes this to store workspaceId + eventIngestUri.
type WorkspaceProvisionedEvent struct {
	WorkspaceID    string    `json:"workspaceId"`
	KlynxOrgID     string    `json:"klynxOrgId"`
	TenantID       string    `json:"tenantId"`
	EventIngestURI string    `json:"eventIngestUri"` // e.g. /events/{workspaceId}/
	ProvisionedAt  time.Time `json:"provisionedAt"`
}
