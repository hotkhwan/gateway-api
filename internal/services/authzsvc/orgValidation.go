// internal/services/authzsvc/orgValidation.go
package authzsvc

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

// GetOwnerCount returns the number of owners for an organization
func (s *OrganizationService) GetOwnerCount(ctx context.Context, tenantId, orgId string) (int, error) {
	log := logger.FromCtx(ctx, "authzsvc", "GetOwnerCount")

	relationships, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to list entity relationships")
		return 0, fmt.Errorf("list entity relationships failed: %w", err)
	}

	ownerCount := 0
	for _, r := range relationships {
		if r.Subject.Type == "user" && r.Relation == "owner" {
			ownerCount++
		}
	}

	log.Debug().Str("orgId", orgId).Int("ownerCount", ownerCount).Msg("owner count retrieved")
	return ownerCount, nil
}

// IsUserOwner checks if a user is an owner of the organization
func (s *OrganizationService) IsUserOwner(ctx context.Context, tenantId, orgId, userId string) (bool, error) {
	log := logger.FromCtx(ctx, "authzsvc", "IsUserOwner")

	relationships, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to list entity relationships")
		return false, fmt.Errorf("list entity relationships failed: %w", err)
	}

	for _, r := range relationships {
		if r.Subject.Type == "user" && r.Subject.ID == userId && r.Relation == "owner" {
			return true, nil
		}
	}

	return false, nil
}

// IsUserAdmin checks if a user has admin relation in the organization
func (s *OrganizationService) IsUserAdmin(ctx context.Context, tenantId, orgId, userId string) (bool, error) {
	log := logger.FromCtx(ctx, "authzsvc", "IsUserAdmin")

	relationships, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to list entity relationships")
		return false, fmt.Errorf("list entity relationships failed: %w", err)
	}

	for _, r := range relationships {
		if r.Subject.Type == "user" && r.Subject.ID == userId && r.Relation == "admin" {
			return true, nil
		}
	}

	return false, nil
}

// GetEffectiveManagerCount returns count of owners + admins that are valid/enabled users
func (s *OrganizationService) GetEffectiveManagerCount(ctx context.Context, tenantId, orgId string) (int, error) {
	log := logger.FromCtx(ctx, "authzsvc", "GetEffectiveManagerCount")

	relationships, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("failed to list entity relationships")
		return 0, fmt.Errorf("list entity relationships failed: %w", err)
	}

	// Collect unique user IDs who are owners or admins
	managerUserIds := make(map[string]bool)
	for _, r := range relationships {
		if r.Subject.Type == "user" {
			if r.Relation == "owner" || r.Relation == "admin" {
				managerUserIds[r.Subject.ID] = true
			}
		}
	}

	if len(managerUserIds) == 0 {
		return 0, nil
	}

	// Check which users are valid/enabled
	validCount := 0
	for userId := range managerUserIds {
		isValid, err := s.IsUserValidAndEnabled(ctx, userId)
		if err != nil {
			log.Warn().Err(err).Str("userId", userId).Msg("failed to check user validity, assuming invalid")
			continue
		}
		if isValid {
			validCount++
		}
	}

	log.Debug().Str("orgId", orgId).Int("validManagerCount", validCount).Msg("effective manager count retrieved")
	return validCount, nil
}

// IsUserValidAndEnabled checks if user exists and is enabled via ID service
func (s *OrganizationService) IsUserValidAndEnabled(ctx context.Context, userId string) (bool, error) {
	users, err := s.idClient.GetUsersByIds(ctx, []string{userId})
	if err != nil {
		return false, nil // User not found, consider invalid
	}
	if len(users) == 0 {
		return false, nil // User not found, consider invalid
	}
	return users[0].Enabled, nil
}

// IsEffectiveManager checks if user is owner or admin AND is valid/enabled
func (s *OrganizationService) IsEffectiveManager(ctx context.Context, tenantId, orgId, userId string) (bool, error) {
	log := logger.FromCtx(ctx, "authzsvc", "IsEffectiveManager")

	// Check if user is owner or admin
	isOwner, err := s.IsUserOwner(ctx, tenantId, orgId, userId)
	if err != nil {
		return false, err
	}
	isAdmin, err := s.IsUserAdmin(ctx, tenantId, orgId, userId)
	if err != nil {
		return false, err
	}
	if !isOwner && !isAdmin {
		return false, nil
	}

	// Check if user is valid/enabled
	isValid, err := s.IsUserValidAndEnabled(ctx, userId)
	if err != nil {
		log.Warn().Err(err).Str("userId", userId).Msg("failed to check user validity, assuming invalid")
		return false, nil
	}

	log.Debug().Str("userId", userId).Str("orgId", orgId).Bool("isEffective", isValid).Msg("effective manager check completed")
	return isValid, nil
}

// CanRemoveOwner validates if an owner can be removed (TWO-TIER CHECK)
func (s *OrganizationService) CanRemoveOwner(ctx context.Context, tenantId, orgId, userId string) (bool, error) {
	log := logger.FromCtx(ctx, "authzsvc", "CanRemoveOwner")

	// Tier 1: Policy level - owners >= 1
	ownerCount, err := s.GetOwnerCount(ctx, tenantId, orgId)
	if err != nil {
		return false, err
	}
	if ownerCount <= 1 {
		log.Warn().
			Str("orgId", orgId).
			Str("userId", userId).
			Int("ownerCount", ownerCount).
			Msg("cannot remove last owner")
		return false, ErrCannotRemoveLastOwner
	}

	// Tier 2: Functional level - effectiveManagers >= 1 (valid/enabled users)
	// After removing this user, check if there are still effective managers
	effectiveCount, err := s.GetEffectiveManagerCount(ctx, tenantId, orgId)
	if err != nil {
		return false, err
	}
	// If removing this user is the only effective manager, block
	isUserEffectiveManager, err := s.IsEffectiveManager(ctx, tenantId, orgId, userId)
	if err != nil {
		return false, err
	}
	if isUserEffectiveManager && effectiveCount <= 1 {
		log.Warn().
			Str("orgId", orgId).
			Str("userId", userId).
			Int("effectiveManagerCount", effectiveCount).
			Msg("cannot remove last effective manager")
		return false, ErrCannotRemoveLastEffectiveManager
	}

	log.Info().
		Str("orgId", orgId).
		Str("userId", userId).
		Msg("owner can be safely removed")
	return true, nil
}
