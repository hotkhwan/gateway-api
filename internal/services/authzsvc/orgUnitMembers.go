package authzsvc

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

type OUAssignUser struct {
	UserId string `json:"userId"`
	Role   string `json:"role"` // member | admin
}

type OURemoveUser struct {
	UserId string `json:"userId"`
}

type OUBulkResult struct {
	UserId  string `json:"userId"`
	Role    string `json:"role,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// internal/services/authzsvc/orgUnitMembers.go

type OUMember struct {
	UserId    string `json:"userId"`
	Role      string `json:"role"`
	Email     string `json:"email,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Enabled   bool   `json:"enabled"`
}

func (s *OrgUnitService) ListMembersOfOU(
	ctx context.Context,
	tenantId string,
	orgId string,
	ouId string,
) ([]OUMember, error) {

	rels, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "orgUnit", ouId)
	if err != nil {
		return nil, err
	}

	// build role map
	roleMap := make(map[string]string)
	for _, r := range rels {
		if r.Subject.Type != "user" {
			continue
		}
		if r.Relation == "member" || r.Relation == "admin" {
			roleMap[r.Subject.ID] = r.Relation
		}
	}

	if len(roleMap) == 0 {
		return []OUMember{}, nil
	}

	userIds := make([]string, 0, len(roleMap))
	for uid := range roleMap {
		userIds = append(userIds, uid)
	}

	// ✅ enrich with Keycloak (เหมือน org ListMembers)
	profiles, err := s.idClient.GetUsersByIds(ctx, userIds)
	profileMap := make(map[string]authgw.UserProfile)
	if err == nil {
		for _, p := range profiles {
			profileMap[p.ID] = p
		}
	}

	out := make([]OUMember, 0, len(userIds))
	for uid, role := range roleMap {
		m := OUMember{UserId: uid, Role: role}
		if p, ok := profileMap[uid]; ok {
			m.Email = p.Email
			m.FirstName = p.FirstName
			m.LastName = p.LastName
			m.Enabled = p.Enabled
		}
		out = append(out, m)
	}

	return out, nil
}

func (s *OrgUnitService) AssignUsersToOU(
	ctx context.Context,
	tenantId string,
	orgId string,
	ouId string,
	callerUserId string,
	users []OUAssignUser,
) ([]OUBulkResult, int, int, error) {

	log := logger.FromCtx(ctx, "authzsvc", "AssignUsersToOU")

	if tenantId == "" || orgId == "" || ouId == "" {
		return nil, 0, 0, ErrInvalidArgs
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
		return nil, 0, 0, err
	}
	if !allowed {
		return nil, 0, 0, ErrForbidden
	}

	// 2️⃣ Verify OU belongs to org
	rels, err := s.authzClient.ListEntityRelationships(
		ctx,
		tenantId,
		"orgUnit",
		ouId,
	)
	if err != nil {
		return nil, 0, 0, err
	}

	belongs := false
	for _, r := range rels {
		if r.Relation == "parentOrg" &&
			r.Subject.Type == "organization" &&
			r.Subject.ID == orgId {
			belongs = true
			break
		}
	}
	if !belongs {
		return nil, 0, 0, errors.New("org unit does not belong to organization")
	}

	// 3️⃣ Get org members
	orgRels, err := s.authzClient.ListEntityRelationships(
		ctx,
		tenantId,
		"organization",
		orgId,
	)
	if err != nil {
		return nil, 0, 0, err
	}

	orgMembers := make(map[string]bool)
	for _, r := range orgRels {
		if r.Subject.Type == "user" &&
			(r.Relation == "member" || r.Relation == "admin") {
			orgMembers[r.Subject.ID] = true
		}
	}

	// 4️⃣ Existing OU relationships
	existing := make(map[string]string)
	for _, r := range rels {
		if r.Subject.Type == "user" &&
			(r.Relation == "member" || r.Relation == "admin") {
			existing[r.Subject.ID] = r.Relation
		}
	}

	results := make([]OUBulkResult, 0)
	tuples := make([]map[string]interface{}, 0)

	inserted := 0
	duplicates := 0

	for _, u := range users {

		if !orgMembers[u.UserId] {
			results = append(results, OUBulkResult{
				UserId:  u.UserId,
				Role:    u.Role,
				Success: false,
				Error:   "user not member of organization",
			})
			continue
		}

		relation := "member"
		if u.Role == "admin" {
			relation = "admin"
		}

		if oldRole, ok := existing[u.UserId]; ok {
			results = append(results, OUBulkResult{
				UserId:  u.UserId,
				Role:    oldRole,
				Success: false,
				Error:   "user already in orgUnit",
			})
			duplicates++
			continue
		}

		tuples = append(tuples, map[string]interface{}{
			"entity": map[string]interface{}{
				"type": "orgUnit",
				"id":   ouId,
			},
			"relation": relation,
			"subject": map[string]interface{}{
				"type": "user",
				"id":   u.UserId,
			},
		})

		results = append(results, OUBulkResult{
			UserId:  u.UserId,
			Role:    relation,
			Success: true,
		})

		inserted++
	}

	if len(tuples) > 0 {
		if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
			return nil, 0, 0, err
		}
	}

	log.Info().
		Int("inserted", inserted).
		Int("duplicates", duplicates).
		Msg("assign OU members complete")

	return results, inserted, duplicates, nil
}

func (s *OrgUnitService) RemoveUsersFromOU(
	ctx context.Context,
	tenantId string,
	orgId string,
	ouId string,
	callerUserId string,
	users []OURemoveUser,
) ([]OUBulkResult, int, error) {

	log := logger.FromCtx(ctx, "authzsvc", "RemoveUsersFromOU")

	if tenantId == "" || orgId == "" || ouId == "" {
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

	// 2️⃣ Verify OU belongs to org
	rels, err := s.authzClient.ListEntityRelationships(
		ctx,
		tenantId,
		"orgUnit",
		ouId,
	)
	if err != nil {
		return nil, 0, err
	}

	belongs := false
	for _, r := range rels {
		if r.Relation == "parentOrg" &&
			r.Subject.Type == "organization" &&
			r.Subject.ID == orgId {
			belongs = true
			break
		}
	}
	if !belongs {
		return nil, 0, errors.New("org unit does not belong to organization")
	}

	// 3️⃣ Build existing membership map
	existing := make(map[string]string)
	for _, r := range rels {
		if r.Subject.Type == "user" &&
			(r.Relation == "member" || r.Relation == "admin") {
			existing[r.Subject.ID] = r.Relation
		}
	}

	results := make([]OUBulkResult, 0)
	removed := 0

	// internal/services/authzsvc/orgUnitMembers.go — RemoveUsersFromOU

	for _, u := range users {
		existingRole, ok := existing[u.UserId]
		if !ok {
			results = append(results, OUBulkResult{
				UserId:  u.UserId,
				Success: false,
				Error:   "user not in orgUnit",
			})
			continue
		}

		// ✅ debug log ดูค่าจริงก่อน delete
		log.Info().
			Str("userId", u.UserId).
			Str("existingRole", existingRole).
			Str("ouId", ouId).
			Msg("attempting delete tuple")

		err := s.authzClient.DeleteSpecificTupleWithRelation(
			ctx, tenantId, "orgUnit", ouId, existingRole, "user", u.UserId,
		)
		if err != nil {
			results = append(results, OUBulkResult{UserId: u.UserId, Success: false, Error: err.Error()})
			continue
		}

		results = append(results, OUBulkResult{UserId: u.UserId, Role: existingRole, Success: true})
		removed++
	}

	log.Info().
		Int("removed", removed).
		Msg("remove OU members complete")

	return results, removed, nil
}
