// internal/services/authzsvc/orgTransferBilling.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

// TransferBillingOwnership transfers billing ownership to another user
// RESTRICTED: Only current billingOwnerId or owners can transfer
func (s *OrganizationService) TransferBillingOwnership(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	newBillingOwnerId string,
) error {
	log := logger.FromCtx(ctx, "authzsvc", "TransferBillingOwnership")

	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	callerUserId = strings.TrimSpace(callerUserId)
	newBillingOwnerId = strings.TrimSpace(newBillingOwnerId)

	if tenantId == "" || orgId == "" || callerUserId == "" || newBillingOwnerId == "" {
		return ErrInvalidInviteArgs
	}

	// 1) Get org for validation
	org, err := s.orgRepo.GetByOrgId(ctx, orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to get organization")
		return fmt.Errorf("get organization failed: %w", err)
	}

	// 2) Check org is not orphaned
	if org.IsOrphaned {
		log.Warn().Str("orgId", orgId).Msg("cannot transfer billing ownership in orphaned organization")
		return ErrOrphanedOrganization
	}

	// 3) Permission check: caller must be EITHER:
	//    a) Current BillingOwnerId, OR
	//    b) An owner
	isCallerOwner, err := s.IsUserOwner(ctx, tenantId, orgId, callerUserId)
	if err != nil {
		return fmt.Errorf("check caller owner status failed: %w", err)
	}

	isCallerBillingOwner := org.BillingOwnerId == callerUserId

	if !isCallerOwner && !isCallerBillingOwner {
		log.Warn().
			Str("callerId", callerUserId).
			Str("orgId", orgId).
			Msg("caller is not billing owner or owner, cannot transfer billing ownership")
		return ErrNotBillingOwnerOrOwner
	}

	// 4) Validate new billing owner:
	//    - Must be a member of the org
	//    - Must be a valid/enabled user
	orgIds, err := s.authzClient.LookupOrganizations(ctx, tenantId, newBillingOwnerId)
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
		log.Warn().
			Str("newBillingOwnerId", newBillingOwnerId).
			Str("orgId", orgId).
			Msg("new billing owner is not a member of the org")
		return ErrInvalidBillingOwner
	}

	// Check if new billing owner is valid/enabled
	isValid, err := s.IsUserValidAndEnabled(ctx, newBillingOwnerId)
	if err != nil {
		log.Warn().Err(err).Str("newBillingOwnerId", newBillingOwnerId).Msg("failed to check user validity")
		return ErrInvalidBillingOwner
	}
	if !isValid {
		log.Warn().
			Str("newBillingOwnerId", newBillingOwnerId).
			Msg("new billing owner is not valid or enabled")
		return ErrInvalidBillingOwner
	}

	// 5) Update BillingOwnerId in MongoDB
	oldBillingOwnerId := org.BillingOwnerId
	org.BillingOwnerId = newBillingOwnerId
	org.UpdatedBy = callerUserId
	org.UpdatedAt = time.Now().UTC()

	if err := s.orgRepo.Update(ctx, orgId, map[string]interface{}{
		"billingOwnerId": newBillingOwnerId,
		"updatedBy":      callerUserId,
		"updatedAt":      org.UpdatedAt,
	}); err != nil {
		log.Error().Err(err).
			Str("orgId", orgId).
			Str("newBillingOwnerId", newBillingOwnerId).
			Msg("failed to update billing owner")
		return fmt.Errorf("update billing owner failed: %w", err)
	}

	// 6) Increment MembershipVersion
	oldVersion := org.MembershipVersion
	newVersion := oldVersion + 1
	updated, err := s.orgRepo.UpdateMembershipVersion(ctx, orgId, oldVersion, newVersion)
	if err != nil {
		log.Error().Err(err).Msg("failed to increment membership version")
		return fmt.Errorf("failed to increment membership version: %w", err)
	}
	if !updated {
		// Race detected
		log.Warn().
			Str("orgId", orgId).
			Int("expectedVersion", oldVersion).
			Msg("concurrent modification detected during billing transfer")
		return ErrConcurrentModification
	}

	// 7) Emit audit log
	log.Info().
		Str("orgId", orgId).
		Str("callerId", callerUserId).
		Str("previousBillingOwnerId", oldBillingOwnerId).
		Str("newBillingOwnerId", newBillingOwnerId).
		Msg("billing ownership transferred")

	return nil
}
