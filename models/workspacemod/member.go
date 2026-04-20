// models/workspacemod/member.go
package workspacemod

import "time"

// WorkspaceMemberRole represents the 4-level workspace-scoped RBAC roles.
type WorkspaceMemberRole string

const (
	RoleOwner    WorkspaceMemberRole = "owner"
	RoleAdmin    WorkspaceMemberRole = "admin"
	RoleOperator WorkspaceMemberRole = "operator"
	RoleViewer   WorkspaceMemberRole = "viewer"
)

// ValidRoles is the ordered set of allowed roles (owner first = highest privilege).
var ValidRoles = []WorkspaceMemberRole{RoleOwner, RoleAdmin, RoleOperator, RoleViewer}

// IsValidRole returns true if the role is one of the four allowed values.
func IsValidRole(r WorkspaceMemberRole) bool {
	for _, v := range ValidRoles {
		if v == r {
			return true
		}
	}
	return false
}

// WorkspaceMember is the response DTO for a workspace member.
// User identity fields are enriched from Keycloak at query time.
type WorkspaceMember struct {
	UserID    string              `json:"userId"`
	Email     string              `json:"email,omitempty"`
	FirstName string              `json:"firstName,omitempty"`
	LastName  string              `json:"lastName,omitempty"`
	Role      WorkspaceMemberRole `json:"role"`
	Enabled   bool                `json:"enabled"`
	CreatedAt time.Time           `json:"createdAt,omitempty"`
}

// WorkspaceInviteRequest is the input for inviting a user to a workspace.
type WorkspaceInviteRequest struct {
	UserID string              `json:"userId"`
	Role   WorkspaceMemberRole `json:"role"`
}
