// internal/services/authzsvc/orgOwnerPromotion.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

// PromoteUserToOwner promotes a member to owner with invariant checks + race protection
func (s *OrganizationService) PromoteUserToOwner(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	targetUserId string,
) error {
	log := logger.FromCtx(ctx, "authzsvc", "PromoteUserToOwner")

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	callerUserId = strings.TrimSpace(callerUserId)
	targetUserId = strings.TrimSpace(targetUserId)

	if tenantId == "" || orgId == "" || callerUserId == "" || targetUserId == "" {
		return ErrInvalidInviteArgs
	}

	// 1) Permission check: caller must be owner
	isCallerOwner, err := s.IsUserOwner(ctx, tenantId, orgId, callerUserId)
	if err != nil {
		return fmt.Errorf("check caller owner status failed: %w", err)
	}
	if !isCallerOwner {
		log.Warn().
			Str("callerId", callerUserId).
			Str("orgId", orgId).
			Msg("caller is not an owner, cannot promote")
		return ErrMustBeOwner
	}

	// 2) Get org for race protection (MembershipVersion)
	org, err := s.orgRepo.GetByOrgId(ctx, orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to get organization")
		return fmt.Errorf("get organization failed: %w", err)
	}

	if org.IsOrphaned {
		log.Warn().Str("orgId", orgId).Msg("cannot promote user in orphaned organization")
		return ErrOrphanedOrganization
	}

	// 3) Validate target user is a member of org
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
		return ErrInviteAlreadyMember
	}

	// Check if user is already an owner
	isAlreadyOwner, err := s.IsUserOwner(ctx, tenantId, orgId, targetUserId)
	if err != nil {
		return fmt.Errorf("check owner status failed: %w", err)
	}
	if isAlreadyOwner {
		return ErrAlreadyOwner
	}

	// 4) Write owner tuple to Permify
	ownerTuple := map[string]any{
		"entity": map[string]any{
			"type": "organization",
			"id":   orgId,
		},
		"relation": "owner",
		"subject": map[string]any{
			"type": "user",
			"id":   targetUserId,
		},
	}

	if err := s.authzClient.WriteTuples(ctx, tenantId, []map[string]any{ownerTuple}); err != nil {
		log.Error().Err(err).
			Str("orgId", orgId).
			Str("userId", targetUserId).
			Msg("failed to write owner tuple")
		return fmt.Errorf("write owner tuple failed: %w", err)
	}

	// 5) Increment MembershipVersion (optimistic locking)
	oldVersion := org.MembershipVersion
	newVersion := oldVersion + 1
	updated, err := s.orgRepo.UpdateMembershipVersion(ctx, org.OrgId, oldVersion, newVersion)
	if err != nil {
		log.Error().Err(err).Msg("failed to increment membership version")
		return fmt.Errorf("failed to increment membership version: %w", err)
	}
	if !updated {
		// Race detected - another mutation occurred
		log.Warn().
			Str("orgId", orgId).
			Int("expectedVersion", oldVersion).
			Msg("concurrent modification detected during promotion")
		return ErrConcurrentModification
	}

	// 6) Emit audit log (if audit service available)
	log.Info().
		Str("orgId", orgId).
		Str("callerId", callerUserId).
		Str("targetId", targetUserId).
		Msg("user promoted to owner")

	return nil
}

// DemoteUserFromOwner demotes owner to member (or admin) with invariant checks
func (s *OrganizationService) DemoteUserFromOwner(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	targetUserId string,
	newRole string, // "member" or "admin" - demote to which role
) error {
	log := logger.FromCtx(ctx, "authzsvc", "DemoteUserFromOwner")

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	callerUserId = strings.TrimSpace(callerUserId)
	targetUserId = strings.TrimSpace(targetUserId)
	newRole = strings.TrimSpace(strings.ToLower(newRole))

	if tenantId == "" || orgId == "" || callerUserId == "" || targetUserId == "" {
		return ErrInvalidInviteArgs
	}

	// Validate demote role
	if newRole != "member" && newRole != "admin" {
		return ErrInvalidDemoteRole
	}

	// 1) Permission check: caller must be owner
	isCallerOwner, err := s.IsUserOwner(ctx, tenantId, orgId, callerUserId)
	if err != nil {
		return fmt.Errorf("check caller owner status failed: %w", err)
	}
	if !isCallerOwner {
		log.Warn().
			Str("callerId", callerUserId).
			Str("orgId", orgId).
			Msg("caller is not an owner, cannot demote")
		return ErrMustBeOwner
	}

	// 2) Get org for race protection (MembershipVersion)
	org, err := s.orgRepo.GetByOrgId(ctx, orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to get organization")
		return fmt.Errorf("get organization failed: %w", err)
	}

	if org.IsOrphaned {
		log.Warn().Str("orgId", orgId).Msg("cannot demote user in orphaned organization")
		return ErrOrphanedOrganization
	}

	// 3) Validate target user is an owner
	isTargetOwner, err := s.IsUserOwner(ctx, tenantId, orgId, targetUserId)
	if err != nil {
		return fmt.Errorf("check owner status failed: %w", err)
	}
	if !isTargetOwner {
		return ErrNotAnOwner
	}

	// 4) TWO-TIER invariant check:
	//    - After demotion, owners >= 1
	//    - After demotion, effectiveManagers >= 1
	ownerCount, err := s.GetOwnerCount(ctx, tenantId, orgId)
	if err != nil {
		return fmt.Errorf("get owner count failed: %w", err)
	}

	// If demoting would leave only 0 owners, block
	if ownerCount <= 1 {
		log.Warn().
			Str("orgId", orgId).
			Str("userId", targetUserId).
			Int("ownerCount", ownerCount).
			Msg("cannot demote last owner")
		return ErrCannotRemoveLastOwner
	}

	// Check effective managers
	effectiveCount, err := s.GetEffectiveManagerCount(ctx, tenantId, orgId)
	if err != nil {
		return fmt.Errorf("get effective manager count failed: %w", err)
	}

	// If demoting the user is the only effective manager and new role is not admin, block
	isTargetEffectiveManager, err := s.IsEffectiveManager(ctx, tenantId, orgId, targetUserId)
	if err != nil {
		return fmt.Errorf("check effective manager status failed: %w", err)
	}
	if isTargetEffectiveManager && effectiveCount <= 1 {
		log.Warn().
			Str("orgId", orgId).
			Str("userId", targetUserId).
			Int("effectiveManagerCount", effectiveCount).
			Msg("cannot demote last effective manager")
		return ErrCannotRemoveLastEffectiveManager
	}

	// 5) Delete owner tuple
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, tenantId, "organization", orgId, "owner", "user", targetUserId,
	); err != nil {
		log.Error().Err(err).
			Str("orgId", orgId).
			Str("userId", targetUserId).
			Msg("failed to delete owner tuple")
		return fmt.Errorf("delete owner tuple failed: %w", err)
	}

	// 6) If newRole == "admin", write admin tuple
	if newRole == "admin" {
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
		if err := s.authzClient.WriteTuples(ctx, tenantId, []map[string]any{adminTuple}); err != nil {
			log.Error().Err(err).
				Str("orgId", orgId).
				Str("userId", targetUserId).
				Msg("failed to write admin tuple")
			return fmt.Errorf("write admin tuple failed: %w", err)
		}
	}

	// 7) Increment MembershipVersion
	oldVersion := org.MembershipVersion
	newVersion := oldVersion + 1
	updated, err := s.orgRepo.UpdateMembershipVersion(ctx, org.OrgId, oldVersion, newVersion)
	if err != nil {
		log.Error().Err(err).Msg("failed to increment membership version")
		return fmt.Errorf("failed to increment membership version: %w", err)
	}
	if !updated {
		// Race detected
		log.Warn().
			Str("orgId", orgId).
			Int("expectedVersion", oldVersion).
			Msg("concurrent modification detected during demotion")
		return ErrConcurrentModification
	}

	// 8) If demoting billing owner, assign new billing owner
	if org.BillingOwnerId == targetUserId {
		newBillingOwnerId, err := s.findFirstOwnerId(ctx, tenantId, orgId)
		if err != nil {
			log.Warn().Err(err).Msg("failed to find new billing owner, keeping orphaned billing")
		} else if newBillingOwnerId != "" {
			org.BillingOwnerId = newBillingOwnerId
			org.UpdatedBy = callerUserId
			org.UpdatedAt = time.Now().UTC()
			// Note: We skip version check here since we just updated it
			if err := s.orgRepo.Update(ctx, orgId, map[string]interface{}{
				"billingOwnerId": newBillingOwnerId,
				"updatedBy":      callerUserId,
				"updatedAt":      org.UpdatedAt,
			}); err != nil {
				log.Error().Err(err).Msg("failed to update billing owner")
			}
		}
	}

	// 9) Emit audit log
	log.Info().
		Str("orgId", orgId).
		Str("callerId", callerUserId).
		Str("targetId", targetUserId).
		Str("newRole", newRole).
		Msg("user demoted from owner")

	return nil
}
