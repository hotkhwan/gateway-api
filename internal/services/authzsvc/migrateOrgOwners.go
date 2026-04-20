// internal/services/authzsvc/migrateOrgOwners.go
package authzsvc

import (
	"context"
	"sort"
	"time"

	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
)

// MigrateExistingOrgs migrates existing organizations to have owners
// Three-step migration logic:
// Step 1: If createdBy exists and user is valid → set as owner
// Step 2: If no createdBy or user invalid → choose oldest admin (deterministic)
// Step 3: No admins → mark as orphaned
func MigrateExistingOrgs(
	ctx context.Context,
	authzClient authzgw.Client,
	idClient *authgw.Client,
	orgRepo *authzrepo.OrgRepo,
	tenantId string,
) error {

	log := logger.FromCtx(ctx, "authzsvc", "MigrateExistingOrgs")

	// 1) Find all organizations
	orgs, err := listAllOrgs(ctx, orgRepo, tenantId)
	if err != nil {
		log.Error().Err(err).Msg("failed to list organizations")
		return err
	}

	migrated := 0
	orphaned := 0

	for _, org := range orgs {
		var chosenUserId string

		// Step 1: If createdBy exists and user is still valid → set as owner
		if org.CreatedBy != "" {
			user, err := getUserById(ctx, idClient, org.CreatedBy)
			if err == nil && user != nil && user.Enabled {
				chosenUserId = org.CreatedBy
				log.Info().
					Str("orgId", org.WorkspaceId).
					Str("userId", chosenUserId).
					Msg("migration: set createdBy as owner")
			} else {
				log.Warn().
					Str("orgId", org.WorkspaceId).
					Str("createdBy", org.CreatedBy).
					Err(err).
					Msg("migration: createdBy user invalid or disabled")
			}
		}

		// Step 2: If no createdBy or user invalid → choose "oldest admin" (deterministic)
		if chosenUserId == "" {
			admins := listAdminsByOrgId(ctx, authzClient, tenantId, org.WorkspaceId)
			if len(admins) > 0 {
				// DETERMINISTIC: sort by userId lexicographically for stable selection
				sort.Strings(admins)
				chosenUserId = admins[0] // first user in sorted list
				log.Info().
					Str("orgId", org.WorkspaceId).
					Str("userId", chosenUserId).
					Int("adminCount", len(admins)).
					Msg("migration: chose oldest admin (first by userId sort) as owner")
			}
		}

		// Step 3: No admins → mark as orphaned
		if chosenUserId == "" {
			org.IsOrphaned = true
			orphaned++
			log.Error().
				Str("orgId", org.WorkspaceId).
				Msg("migration: org marked as orphaned - no admins found, requires manual claim")
			// Update org in MongoDB
			if err := updateOrg(ctx, orgRepo, org); err != nil {
				log.Error().Err(err).Str("orgId", org.WorkspaceId).Msg("failed to update org as orphaned")
			}
			continue
		}

		// Write owner tuple to Permify
		ownerTuple := map[string]any{
			"entity": map[string]any{
				"type": "organization",
				"id":   org.WorkspaceId,
			},
			"relation": "owner",
			"subject": map[string]any{
				"type": "user",
				"id":   chosenUserId,
			},
		}

		if err := writeTuples(ctx, authzClient, tenantId, []map[string]any{ownerTuple}); err != nil {
			log.Error().
				Err(err).
				Str("orgId", org.WorkspaceId).
				Str("userId", chosenUserId).
				Msg("migration: failed to write owner tuple")
			continue
		}

		// Update org in MongoDB
		org.BillingOwnerId = chosenUserId
		org.MembershipVersion = 1 // initialize version
		org.UpdatedAt = time.Now().UTC()

		if err := updateOrg(ctx, orgRepo, org); err != nil {
			log.Error().
				Err(err).
				Str("orgId", org.WorkspaceId).
				Msg("migration: failed to update org")
			continue
		}

		migrated++
	}

	log.Info().
		Int("total", len(orgs)).
		Int("migrated", migrated).
		Int("orphaned", orphaned).
		Msg("migration completed")

	return nil
}

// listAllOrgs lists all organizations
func listAllOrgs(ctx context.Context, repo *authzrepo.OrgRepo, tenantId string) ([]*authzmod.Organization, error) {
	orgs, err := repo.ListAll(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	// Convert []authzmod.Organization to []*authzmod.Organization
	result := make([]*authzmod.Organization, len(orgs))
	for i := range orgs {
		result[i] = &orgs[i]
	}
	return result, nil
}

// getUserById retrieves a user by ID
func getUserById(ctx context.Context, idClient *authgw.Client, userId string) (*struct {
	ID      string
	Enabled bool
}, error) {
	users, err := idClient.GetUsersByIds(ctx, []string{userId})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &struct {
		ID      string
		Enabled bool
	}{
		ID:      users[0].ID,
		Enabled: users[0].Enabled,
	}, nil
}

// listAdminsByOrgId returns all admin user IDs for an organization
func listAdminsByOrgId(ctx context.Context, authzClient authzgw.Client, tenantId, orgId string) []string {
	// Query Permify for organization:orgId#admin@user:*
	// Return slice of user IDs
	relationships, err := authzClient.ListEntityRelationships(ctx, tenantId, "organization", orgId)
	if err != nil {
		return []string{}
	}

	var adminUserIds []string
	for _, r := range relationships {
		if r.Relation == "admin" && r.Subject.Type == "user" {
			adminUserIds = append(adminUserIds, r.Subject.ID)
		}
	}
	return adminUserIds
}

// writeTuples writes tuples to Permify
func writeTuples(ctx context.Context, authzClient authzgw.Client, tenantId string, tuples []map[string]any) error {
	return authzClient.WriteTuples(ctx, tenantId, tuples)
}

// updateOrg updates organization in MongoDB
func updateOrg(ctx context.Context, repo *authzrepo.OrgRepo, org *authzmod.Organization) error {
	return repo.Update(ctx, org.WorkspaceId, map[string]interface{}{
		"billingOwnerId":    org.BillingOwnerId,
		"isOrphaned":        org.IsOrphaned,
		"membershipVersion": org.MembershipVersion,
		"updatedAt":         org.UpdatedAt,
	})
}
