// controllers/usrapi/batch.go
package usrapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// BatchGetUsers godoc
// @Summary      Batch get users by IDs
// @Description  Get multiple Keycloak users by userIds (POST body)
// @Tags         Users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param body body usrmod.BatchGetUsersRequest true "User IDs"
// @Success 200 {object} usrmod.PaginationSuccessUsers
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /users/batch [post]
func BatchGetUsers(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.usrapi", "BatchGetUsers", "usrapi", "BatchGetUsers")
	defer end()

	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return httputil.FailUnauthorized(c, "Missing Authorization header")
	}

	var req usrmod.BatchGetUsersRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "Invalid JSON body")
	}

	if len(req.UserIds) == 0 {
		return httputil.FailBadRequest(c, "userIds is required")
	}

	if len(req.UserIds) > 200 {
		return httputil.FailBadRequest(c, "userIds too many (max 200)")
	}

	// trim + dedupe
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(req.UserIds))
	for _, id := range req.UserIds {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	result, err := usrsvc.GetUsersByIds(ctx, authHeader, ids)
	if err != nil {
		if strings.Contains(err.Error(), "token") {
			log.Error().Err(err).Msg("❌ BatchGetUsers unauthorized")
			return httputil.FailUnauthorized(c, err.Error())
		}
		log.Error().Err(err).Msg("❌ BatchGetUsers failed")
		return httputil.FailInternal(c, err.Error())
	}

	log.Debug().Int("count", len(result.Details)).Msg("✅ BatchGetUsers fetched")

	return gmod.SendPagination(c, result.Details, result.Pagination)
}
