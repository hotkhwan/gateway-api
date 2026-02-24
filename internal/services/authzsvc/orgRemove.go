// internal/services/authzsvc/orgRemove.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

func (s *OrganizationService) RemoveUser(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	targetUserId string,
) error {
	log := logger.FromCtx(ctx, "authzsvc", "OrganizationService.RemoveUser")

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	callerUserId = strings.TrimSpace(callerUserId)
	targetUserId = strings.TrimSpace(targetUserId)

	if tenantId == "" || orgId == "" || callerUserId == "" || targetUserId == "" {
		return ErrInvalidInviteArgs
	}

	// 1) Permission check
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", orgId, "manage", "user", callerUserId,
	)
	if err != nil {
		return fmt.Errorf("authz check manage failed: %w", err)
	}
	if !allowed {
		return ErrForbiddenInvite
	}

	// 2) Check membership
	orgIds, err := s.authzClient.LookupOrganizations(ctx, tenantId, targetUserId)
	if err != nil {
		return fmt.Errorf("lookup organizations failed: %w", err)
	}
	isMember := false
	for _, id := range orgIds {
		if id == orgId {
			isMember = true
			break
		}
	}
	if !isMember {
		return ErrRemoveNotMember
	}

	// 3) Delete member tuple
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "member", "user", targetUserId,
	); err != nil {
		log.Error().Err(err).Str("userId", targetUserId).Msg("❌ delete member tuple failed")
		return fmt.Errorf("delete member tuple failed: %w", err)
	}

	// 4) Delete admin tuple (best-effort — user อาจเป็นแค่ member ไม่มี admin tuple)
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "admin", "user", targetUserId,
	); err != nil {
		log.Warn().Err(err).Str("userId", targetUserId).Msg("⚠️ delete admin tuple failed (ignored)")
	}

	log.Info().Str("userId", targetUserId).Str("orgId", orgId).Msg("✅ User removed from org")
	return nil
}
