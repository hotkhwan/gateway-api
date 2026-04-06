// controllers/memapi/getByGroupId.go
package memapi

import (
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/internal/services/memsvc"
	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetMemberByUserID godoc
// @Summary Get member by ID
// @Description Return member detail by ID
// @Tags members
// @Produce  json
// @Param userId path string true "member ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members/{groupId} [get]
// @Security BearerAuth
func MemberGetByGroupID(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.memapi", "memapi.MemberGetByGroupID", "memapi", "MemberGetByGroupID")
	defer end()

	groupId := strings.TrimSpace(c.Params("groupId"))
	if groupId == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid groupId")
	}

	//หา group tree
	groups, err := grpsvc.GetAllGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get groups")
		return httputil.FailInternal(c, "GET_GROUPS_FAILED", err.Error())
	}

	tree := grpsvc.BuildGroupTree(ctx, groups, nil)

	selected := memsvc.FindInGroupTree(ctx, tree, groupId)
	log.Info().Interface("selected", selected).Msg("🔎 Selected group in tree")

	member, err := memsvc.MembersGetByGroupID(ctx, selected)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "member not found")
		}
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ MemberGetByGroupID failed")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_MEMBER")
	}
	// 1. รวบรวม userIDs ทั้งหมดจาก member
	userIDsMap := make(map[string]bool)
	for _, g := range member {
		for _, uid := range g.UserIDs {
			if uid != "" {
				userIDsMap[uid] = true
			}
		}
	}

	userIDs := make([]string, 0, len(userIDsMap))
	for uid := range userIDsMap {
		userIDs = append(userIDs, uid)
	}

	// 2. ยิงหาข้อมูล User จาก Keycloak มาเก็บไว้ใน Map (Cache)
	adminToken, err := authutil.GetAdminAccessToken()
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get admin token")
		return nil
	}

	userDataMap := make(map[string]usrmod.KeycloakUser)

	result, err := usrsvc.GetUsersByIds(ctx, adminToken, userIDs)
	if err != nil {
		if strings.Contains(err.Error(), "token") {
			log.Error().Err(err).Msg("❌ [ListUsers] Failed to get users by ids")
			return httputil.FailUnauthorized(c, err.Error())
		}
		return httputil.FailInternal(c, err.Error())
	}

	for _, u := range result.Details {
		userDataMap[u.ID] = u
	}

	var response []memmod.MemberDetailResponse

	for _, g := range member {
		var members []memmod.MemberDetail

		for _, uid := range g.UserIDs {
			m := memmod.MemberDetail{
				UserID: uid,
			}

			if u, ok := userDataMap[uid]; ok {
				m.FirstName = u.FirstName
				m.LastName = u.LastName
				m.Role = u.Role
				m.Enabled = u.Enabled
			}
			members = append(members, m)
		}

		// ใส่ข้อมูลลงใน Response ของแต่ละ Group
		response = append(response, memmod.MemberDetailResponse{
			GroupID: g.GroupID,
			Users:   members,
			Status:  true,
		})
	}

	return httputil.Ok(c, response)

}
