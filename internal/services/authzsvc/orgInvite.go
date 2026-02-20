// internal/services/authzsvc/orgInvite.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
)

func normalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return "member"
	}
	return role
}

func (s *OrganizationService) InviteUser(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	targetUserId string,
	role string,
) error {

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	callerUserId = strings.TrimSpace(callerUserId)
	targetUserId = strings.TrimSpace(targetUserId)
	role = normalizeRole(role)

	if tenantId == "" || orgId == "" || callerUserId == "" || targetUserId == "" {
		return ErrInvalidInviteArgs
	}

	if role != "member" && role != "admin" {
		return ErrInviteRoleNotAllowed
	}

	// Block self escalation (caller invites himself as admin)
	if callerUserId == targetUserId && role == "admin" {
		return ErrInviteSelfAdminBlocked
	}

	// 1) Permission check: caller must manage organization
	schemaVersion := strings.TrimSpace(config.CurrentSchemaVersion)
	if schemaVersion == "" {
		return fmt.Errorf("current schema version is empty")
	}

	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx,
		tenantId,
		schemaVersion,
		"organization",
		orgId,
		"manage",
		"user",
		callerUserId,
	)
	if err != nil {
		return fmt.Errorf("authz check manage failed: %w", err)
	}
	if !allowed {
		return ErrForbiddenInvite
	}

	// 2) Prevent duplicate membership
	// NOTE: lookup by permission "view" will return orgs where user can view (member/admin)
	orgIds, err := s.authzClient.LookupOrganizations(ctx, tenantId, targetUserId)
	if err != nil {
		return fmt.Errorf("lookup organizations failed: %w", err)
	}
	for _, id := range orgIds {
		if id == orgId {
			return ErrInviteAlreadyMember
		}
	}

	// 3) Write tuples (member always, plus admin if role=admin)
	tuples := make([]map[string]any, 0, 2)

	memberTuple := map[string]any{
		"entity": map[string]any{
			"type": "organization",
			"id":   orgId,
		},
		"relation": "member",
		"subject": map[string]any{
			"type": "user",
			"id":   targetUserId,
		},
	}
	tuples = append(tuples, memberTuple)

	if role == "admin" {
		adminTuple := map[string]any{
			"entity": map[string]any{
				"type": "organization",
				"id":   orgId,
			},
			"relation": "admin",
			"subject": map[string]any{
				"type": "user",
				"id":   targetUserId,
			},
		}
		tuples = append(tuples, adminTuple)
	}

	if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
		// no mongo change here; pure permify grant
		// caller can retry safely (idempotent-ish but duplicate may appear depending on permify behavior)
		return fmt.Errorf("write tuples failed: %w", err)
	}

	// Optional: you can emit audit here if you have auditSvc injected.
	_ = authzgw.Relationship{} // keep import usage if you later refactor to typed tuples

	return nil
}
