// internal/services/workspacesvc/memberSvc.go
package workspacesvc

import (
	"context"
	"errors"
	"fmt"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

var (
	ErrMemberNotFound    = errors.New("member not found in workspace")
	ErrInvalidRole       = errors.New("invalid workspace role")
	ErrCannotRemoveOwner = errors.New("cannot remove workspace owner — transfer ownership first")
)

// WorkspaceMemberService manages workspace membership via Permify tuples.
// Membership storage is Permify-only (no separate Mongo collection).
// User display fields (name, email) are enriched from Keycloak via authgw.
type WorkspaceMemberService struct {
	authz    authzgw.Client
	idClient *authgw.Client
}

// NewWorkspaceMemberService creates a WorkspaceMemberService.
// idClient may be nil — member listing will return userId only without enrichment.
func NewWorkspaceMemberService(authz authzgw.Client, idClient *authgw.Client) *WorkspaceMemberService {
	return &WorkspaceMemberService{authz: authz, idClient: idClient}
}

func permifyTenant() string {
	return config.PermifyTenantID
}

// InviteMember writes a Permify tuple granting role to userId on workspaceId.
// If the user already has a tuple on this workspace, the old tuple is replaced.
func (s *WorkspaceMemberService) InviteMember(ctx context.Context, workspaceID, userID string, role workspacemod.WorkspaceMemberRole) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.InviteMember",
		"workspacesvc", "InviteMember",
	)
	defer end()

	if !workspacemod.IsValidRole(role) {
		return ErrInvalidRole
	}

	// Remove any existing role tuple for this user first (idempotent role change).
	for _, r := range workspacemod.ValidRoles {
		_ = s.authz.DeleteSpecificTupleWithRelation(ctx, permifyTenant(),
			"workspace", workspaceID, string(r), "user", userID)
	}

	tuples := []map[string]interface{}{
		{
			"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
			"relation": string(role),
			"subject":  map[string]interface{}{"type": "user", "id": userID},
		},
	}
	if err := s.authz.WriteTuples(ctx, permifyTenant(), tuples); err != nil {
		return fmt.Errorf("workspacesvc: write member tuple: %w", err)
	}

	log.Info().Str("workspaceId", workspaceID).Str("userId", userID).Str("role", string(role)).Msg("[workspacesvc] member invited")
	return nil
}

// RemoveMember deletes the Permify tuple for the given user on the workspace.
func (s *WorkspaceMemberService) RemoveMember(ctx context.Context, workspaceID, userID string) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.RemoveMember",
		"workspacesvc", "RemoveMember",
	)
	defer end()

	// Determine current role before removing
	rels, err := s.authz.ListEntityRelationships(ctx, permifyTenant(), "workspace", workspaceID)
	if err != nil {
		return fmt.Errorf("workspacesvc: list relationships: %w", err)
	}

	for _, rel := range rels {
		if rel.Subject.ID == userID {
			if rel.Relation == string(workspacemod.RoleOwner) {
				return ErrCannotRemoveOwner
			}
			break
		}
	}

	removed := false
	for _, r := range workspacemod.ValidRoles {
		if err := s.authz.DeleteSpecificTupleWithRelation(ctx, permifyTenant(),
			"workspace", workspaceID, string(r), "user", userID); err == nil {
			removed = true
		}
	}

	if !removed {
		return ErrMemberNotFound
	}

	log.Info().Str("workspaceId", workspaceID).Str("userId", userID).Msg("[workspacesvc] member removed")
	return nil
}

// ListMembers returns all workspace members with roles, enriched with Keycloak user info.
func (s *WorkspaceMemberService) ListMembers(ctx context.Context, tenantID, workspaceID string) ([]workspacemod.WorkspaceMember, error) {
	ctx, end, _ := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.ListMembers",
		"workspacesvc", "ListMembers",
	)
	defer end()

	rels, err := s.authz.ListEntityRelationships(ctx, permifyTenant(), "workspace", workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspacesvc: list relationships: %w", err)
	}

	// Build userId → role map
	roleMap := make(map[string]workspacemod.WorkspaceMemberRole)
	var userIDs []string
	for _, rel := range rels {
		if rel.Subject.Type != "user" {
			continue
		}
		uid := rel.Subject.ID
		if workspacemod.IsValidRole(workspacemod.WorkspaceMemberRole(rel.Relation)) {
			roleMap[uid] = workspacemod.WorkspaceMemberRole(rel.Relation)
			userIDs = append(userIDs, uid)
		}
	}

	// Enrich with Keycloak user profiles
	profileMap := make(map[string]authgw.UserProfile)
	if s.idClient != nil && len(userIDs) > 0 {
		profiles, err := s.idClient.GetUsersByIds(ctx, userIDs)
		if err == nil {
			for _, p := range profiles {
				profileMap[p.ID] = p
			}
		}
	}

	members := make([]workspacemod.WorkspaceMember, 0, len(userIDs))
	for _, uid := range userIDs {
		m := workspacemod.WorkspaceMember{
			UserID: uid,
			Role:   roleMap[uid],
		}
		if p, ok := profileMap[uid]; ok {
			m.Email = p.Email
			m.FirstName = p.FirstName
			m.LastName = p.LastName
			m.Enabled = p.Enabled
		}
		members = append(members, m)
	}

	return members, nil
}

// ChangeRole updates the Permify tuple to a new role for an existing member.
func (s *WorkspaceMemberService) ChangeRole(ctx context.Context, workspaceID, userID string, newRole workspacemod.WorkspaceMemberRole) error {
	// InviteMember replaces existing tuple — reuse it
	return s.InviteMember(ctx, workspaceID, userID, newRole)
}
