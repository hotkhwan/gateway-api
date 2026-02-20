// controllers/authznewapi/orgInvite.go
package authznewapi

import (
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// ListMembers godoc
// @Summary List organization members
// @Description Return list of members in active organization (admin only)
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param X-Active-Org header string true "Active Organization ID"
// @Success 200 {object} gmod.ApiSuccessResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/users/members [get]
func (ctrl *OrganizationController) ListMembers(c *fiber.Ctx) error {

	ctx := c.UserContext()

	log := logger.FromCtx(ctx, "controller", "ListMembers")

	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	log.Debug().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("callerUserId", callerUserId).
		Msg("list members request")

	members, err := ctrl.service.ListMembers(
		ctx,
		tenantId,
		orgId,
		callerUserId,
	)
	if err != nil {

		log.Warn().
			Err(err).
			Msg("list members failed")

		return c.Status(403).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeForbidden,
			Message: err.Error(),
			Status:  false,
		})
	}

	log.Debug().
		Int("count", len(members)).
		Msg("list members success")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"status":  true,
		"details": members,
	})
}
