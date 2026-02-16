// controllers/usrapi/batch.go
package usrapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/usrsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
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
func BatchGetUsers(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/usrapi",
		"users.BatchGetUsers",
		"usrapi",
		"BatchGetUsers",
	)
	defer span.End()

	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(gmod.ErrorResponse{
			Code:    "UNAUTHORIZED",
			Message: "Missing Authorization header",
			Status:  false,
		})
	}

	var req usrmod.BatchGetUsersRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code:    "BAD_REQUEST",
			Message: "Invalid JSON body",
			Status:  false,
		})
	}

	if len(req.UserIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code:    "BAD_REQUEST",
			Message: "userIds is required",
			Status:  false,
		})
	}

	if len(req.UserIds) > 200 {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorResponse{
			Code:    "BAD_REQUEST",
			Message: "userIds too many (max 200)",
			Status:  false,
		})
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
			return c.Status(fiber.StatusUnauthorized).JSON(gmod.ErrorResponse{
				Code:    "UNAUTHORIZED",
				Message: err.Error(),
				Status:  false,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.ErrorResponse{
			Code:    "BATCH_GET_USERS_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return gmod.SendPagination(c, result.Details, result.Pagination)
}
