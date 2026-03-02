// internal/services/authzsvc/orgListMembers.go
package authzsvc

import (
	"context"
	"sort"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

type OrgMember struct {
	UserId    string `json:"userId"`
	Role      string `json:"role"`         // "admin", "member" (owner shows as admin with isOwner flag)
	IsOwner   bool   `json:"isOwner"`      // NEW: true if has owner relation
	IsAdmin   bool   `json:"isAdmin"`      // NEW: true if has admin relation explicitly (NOT implied from owner)
	IsBillingOwner bool `json:"isBillingOwner"` // NEW: true if is billing owner
	Email     string `json:"email,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Enabled   bool   `json:"enabled"`
}

func (s *OrganizationService) ListMembers(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
	page int,
	size int,
	search string,
	sortField string,
	sortOrder string,
) ([]OrgMember, int, error) {

	log := logger.FromCtx(ctx, "authzsvc", "listMembers")

	if tenantId == "" || orgId == "" || callerUserId == "" {
		return nil, 0, ErrInvalidArgs
	}

	// 1️⃣ Permission check
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx,
		tenantId,
		config.CurrentSchemaVersion,
		"organization",
		orgId,
		"manage",
		"user",
		callerUserId,
	)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, ErrForbidden
	}

	// 2️⃣ Get relationships
	relationships, err := s.authzClient.ListEntityRelationships(
		ctx,
		tenantId,
		"organization",
		orgId,
	)
	if err != nil {
		return nil, 0, err
	}

	ownerSet := make(map[string]bool)   // NEW
	adminSet := make(map[string]bool)
	memberSet := make(map[string]bool)

	for _, r := range relationships {
		if r.Subject.Type != "user" {
			continue
		}
		if r.Relation == "owner" {      // NEW
			ownerSet[r.Subject.ID] = true
			memberSet[r.Subject.ID] = true
			// IMPORTANT: do NOT set adminSet - isAdmin means has admin relation explicitly
		}
		if r.Relation == "admin" {
			adminSet[r.Subject.ID] = true
			memberSet[r.Subject.ID] = true
		}
		if r.Relation == "member" {
			memberSet[r.Subject.ID] = true
		}
	}

	if len(memberSet) == 0 {
		return []OrgMember{}, 0, nil
	}

	userIds := make([]string, 0, len(memberSet))
	for id := range memberSet {
		userIds = append(userIds, id)
	}

	// 3️⃣ Fetch profiles
	profiles, err := s.idClient.GetUsersByIds(ctx, userIds)
	if err != nil {
		return nil, 0, err
	}

	profileMap := make(map[string]authgw.UserProfile)
	for _, p := range profiles {
		profileMap[p.ID] = p
	}

	// NEW: Get org to check billing owner
	org, _ := s.orgRepo.GetByOrgId(ctx, orgId)

	result := make([]OrgMember, 0, len(userIds))

	for _, userId := range userIds {

		role := "member"
		// role = "admin" if user is admin OR owner (owners get management rights)
		if adminSet[userId] || ownerSet[userId] {
			role = "admin"
		}

		member := OrgMember{
			UserId:        userId,
			Role:         role,
			IsOwner:      ownerSet[userId],                      // has owner relation
			IsAdmin:      adminSet[userId],                      // has admin relation (not implied)
			IsBillingOwner: org != nil && org.BillingOwnerId == userId, // is billing owner
		}

		if p, ok := profileMap[userId]; ok {
			member.Email = p.Email
			member.FirstName = p.FirstName
			member.LastName = p.LastName
			member.Enabled = p.Enabled
		}

		result = append(result, member)
	}

	// 🔍 Search filter
	if search != "" {
		search = strings.ToLower(search)
		filtered := make([]OrgMember, 0)
		for _, m := range result {
			if strings.Contains(strings.ToLower(m.Email), search) ||
				strings.Contains(strings.ToLower(m.FirstName), search) ||
				strings.Contains(strings.ToLower(m.LastName), search) {
				filtered = append(filtered, m)
			}
		}
		result = filtered
	}

	totalRecords := len(result)

	// 🔥 Sorting
	sort.Slice(result, func(i, j int) bool {

		asc := strings.ToLower(sortOrder) != "desc"

		var less bool

		switch sortField {
		case "email":
			less = result[i].Email < result[j].Email
		case "firstName":
			less = result[i].FirstName < result[j].FirstName
		case "lastName":
			less = result[i].LastName < result[j].LastName
		default:
			less = result[i].FirstName < result[j].FirstName
		}

		if asc {
			return less
		}
		return !less
	})

	// 🔥 Pagination
	if size <= 0 {
		size = 20
	}
	if page <= 0 {
		page = 1
	}

	start := (page - 1) * size
	if start > totalRecords {
		return []OrgMember{}, totalRecords, nil
	}

	end := start + size
	if end > totalRecords {
		end = totalRecords
	}

	result = result[start:end]

	log.Info().
		Int("total", totalRecords).
		Int("page", page).
		Int("size", size).
		Msg("list members success")

	return result, totalRecords, nil
}
