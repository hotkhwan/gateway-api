// controllers/usrapi/list.go
package usrapi

import (
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

func parseUserIds(c *fiber.Ctx) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)

	// 1) CSV: userIds=1,2,3
	if csv := strings.TrimSpace(c.Query("userIds")); csv != "" {
		for _, p := range strings.Split(csv, ",") {
			id := strings.TrimSpace(p)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}

	// 2) Repeat: userId=1&userId=2
	args := c.Context().QueryArgs()
	args.VisitAll(func(k, v []byte) {
		key := strings.TrimSpace(string(k))
		if key != "userId" {
			return
		}
		id := strings.TrimSpace(string(v))
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	})

	return out
}

// ListUsers godoc
// @Summary      List Keycloak users
// @Description  Get list of users from Keycloak (supports userIds filter)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param userIds query string false "Comma-separated user IDs (e.g. userIds=id1,id2,id3)"
// @Param userId query string false "Repeatable user ID (e.g. userId=id1&userId=id2)"
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page"
// @Param perPage query int false "Items per page (alias of perPages)"
// @Param search query string false "Search term"
// @Param sortField query string false "Sort field: username|email|firstName|lastName|createdAt" default(username)
// @Param sortOrder query string false "Sort order: asc|desc" default(desc)
// @Success 200 {object} usrmod.PaginationSuccessUsers
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router       /users [get]
func ListUsers(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.usrapi", "ListUsers", "usrapi", "ListUsers")
	defer end()

	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return httputil.FailUnauthorized(c, "Missing Authorization header")
	}

	userIds := parseUserIds(c)
	if len(userIds) > 0 {
		// guardrail: กันยิง KC หนักเกิน
		if len(userIds) > 200 {
			return httputil.FailBadRequest(c, "userIds too many (max 200)")
		}

		result, err := usrsvc.GetUsersByIds(ctx, authHeader, userIds)
		if err != nil {
			if strings.Contains(err.Error(), "token") {
				log.Error().Err(err).Msg("❌ [ListUsers] Failed to get users by ids")
				return httputil.FailUnauthorized(c, err.Error())
			}
			log.Error().Err(err).Msg("❌ [ListUsers] GetUsersByIds failed")
			return httputil.FailInternal(c, err.Error())
		}

		log.Debug().Int("count", len(result.Details)).Msg("✅ [ListUsers] users by ids fetched")

		// รักษา contract เดิม: ส่งแบบ pagination
		return gmod.SendPagination(c, result.Details, gmod.Pagination{
			Page:         1,
			PerPages:     len(result.Details),
			TotalRecords: len(result.Details),
			TotalPages:   1,
			SortField:    "userIds",
			SortOrder:    "asc",
		})
	}

	// --- เดิมทั้งหมด (pagination/search/sort) ---
	page := c.QueryInt("page", 1)

	perPages := c.QueryInt("perPages", 0)
	if perPages <= 0 {
		if v := c.Query("perPage"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				perPages = n
			}
		}
	}
	if perPages <= 0 {
		perPages = 10
	}
	if perPages > 200 {
		perPages = 200
	}

	search := c.Query("search", "")
	sortField := strings.ToLower(c.Query("sortField", "username"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	validSortFields := map[string]bool{
		"username":  true,
		"email":     true,
		"firstName": true,
		"lastName":  true,
		"roles":     true,
		"createdAt": true,
	}
	if !validSortFields[sortField] {
		sortField = "username"
	}

	result, err := usrsvc.ListUsers(ctx, authHeader, page, perPages, search, sortField, sortOrder)
	if err != nil {
		if strings.Contains(err.Error(), "token") {
			log.Error().Err(err).Msg("❌ [ListUsers] Failed to user list")
			return httputil.FailUnauthorized(c, err.Error())
		}
		log.Error().Err(err).Msg("❌ [ListUsers] ListUsers failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Debug().Int("count", len(result.Details)).Msg("✅ [ListUsers] users fetched")

	return gmod.SendPagination(c, result.Details, result.Pagination)
}
