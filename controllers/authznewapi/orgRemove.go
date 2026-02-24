// controllers/authznewapi/orgRemove.go
package authznewapi

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// RemoveMembers godoc
// @Summary Remove users from organization
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body InviteUsersRequest true "payload"
// @Success 200 {object} map[string]any
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/users/remove [post]
func (ctrl *OrganizationController) RemoveMembers(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	var body InviteUsersRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}
	if len(body.Users) == 0 {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "users required",
			Status:  false,
		})
	}

	results := make([]map[string]any, 0, len(body.Users))
	removed := 0
	notMember := 0
	errorCount := 0

	for _, u := range body.Users {
		err := ctrl.service.RemoveUser(ctx, tenantId, orgId, callerUserId, u.UserId)

		result := map[string]any{"userId": u.UserId}
		if err != nil {
			result["success"] = false
			result["error"] = err.Error()
			if err == authzsvc.ErrRemoveNotMember {
				notMember++
			} else {
				errorCount++
			}
		} else {
			result["success"] = true
			removed++
		}
		results = append(results, result)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": fmt.Sprintf("removed %d, not_member %d, error %d", removed, notMember, errorCount),
		"status":  true,
		"details": results,
	})
}
