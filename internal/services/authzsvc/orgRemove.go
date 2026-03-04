// internal/services/authzsvc/orgRemove.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"go.mongodb.org/mongo-driver/bson"
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

	// NEW: Get organization for race protection (MembershipVersion)
	org, err := s.orgRepo.GetByOrgId(ctx, orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to get organization")
		return fmt.Errorf("get organization failed: %w", err)
	}

	if org.IsOrphaned {
		log.Warn().Str("orgId", orgId).Msg("cannot remove user from orphaned organization")
		return ErrOrphanedOrganization
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

	// NEW: Two-tier invariant check before delete
	isOwner, err := s.IsUserOwner(ctx, tenantId, orgId, targetUserId)
	if err != nil {
		return fmt.Errorf("check owner status failed: %w", err)
	}
	if isOwner {
		canRemove, err := s.CanRemoveOwner(ctx, tenantId, orgId, targetUserId)
		if err != nil {
			return err
		}
		if !canRemove {
			return ErrCannotRemoveLastOwner
		}
	}

	// 3) Delete owner tuple if exists
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "owner", "user", targetUserId,
	); err != nil {
		log.Warn().Err(err).Str("userId", targetUserId).Msg("delete owner tuple failed (ignored)")
	}

	// 4) Delete admin tuple if exists (best-effort — user อาจเป็นแค่ member ไม่มี admin tuple)
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "admin", "user", targetUserId,
	); err != nil {
		log.Warn().Err(err).Str("userId", targetUserId).Msg("delete admin tuple failed (ignored)")
	}

	// 5) Delete member tuple
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "member", "user", targetUserId,
	); err != nil {
		log.Error().Err(err).Str("userId", targetUserId).Msg("❌ delete member tuple failed")
		return fmt.Errorf("delete member tuple failed: %w", err)
	}

	// NEW: Increment MembershipVersion for race protection (optimistic locking)
	oldVersion := org.MembershipVersion
	newVersion := oldVersion + 1
	updated, err := s.orgRepo.UpdateMembershipVersion(ctx, orgId, oldVersion, newVersion)
	if err != nil {
		log.Error().Err(err).Msg("failed to increment membership version")
		return fmt.Errorf("failed to increment membership version: %w", err)
	}
	if !updated {
		// Race detected - another mutation occurred
		log.Warn().
			Str("orgId", orgId).
			Int("expectedVersion", oldVersion).
			Msg("concurrent modification detected")
		return ErrConcurrentModification
	}

	// NEW: Also update billing owner if removed user was billing owner
	if org.BillingOwnerId == targetUserId {
		// Find another owner to become billing owner
		newBillingOwnerId, err := s.findFirstOwnerId(ctx, tenantId, orgId)
		if err != nil {
			log.Warn().Err(err).Msg("failed to find new billing owner, keeping orphaned billing")
		} else if newBillingOwnerId != "" {
			org.BillingOwnerId = newBillingOwnerId
			// Note: We skip version check here since we just updated it
			if err := s.orgRepo.Update(ctx, orgId, bson.M{
				"billingOwnerId": newBillingOwnerId,
				"updatedBy":      callerUserId,
				"updatedAt":      time.Now().UTC(),
			}); err != nil {
				log.Error().Err(err).Msg("failed to update billing owner")
			}
		}
	}

	log.Info().Str("userId", targetUserId).Str("orgId", orgId).Msg("✅ User removed from org")
	return nil
}

// findFirstOwnerId returns the first owner user ID for an organization
func (s *OrganizationService) findFirstOwnerId(ctx context.Context, tenantId, orgId string) (string, error) {
	relationships, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		return "", err
	}

	for _, r := range relationships {
		if r.Subject.Type == "user" && r.Relation == "owner" {
			return r.Subject.ID, nil
		}
	}

	return "", nil
}
