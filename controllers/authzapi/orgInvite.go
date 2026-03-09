// controllers/authzapi/orgInvite.go
package authzapi

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type InviteUsersRequest struct {
	Users []struct {
		UserId string `json:"userId"`
		Role   string `json:"role"`
	} `json:"users"`
}

// Invite godoc
// @Summary Invite user to organization
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "orgId"
// @Param body body gmod.InviteUserRequest true "payload"
// @Success 200 {object} gmod.InviteUserResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/{id}/invite [post]
func (ctrl *OrganizationController) Invite(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "OrganizationController.Invite", "authzapi", "Invite")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	var body InviteUsersRequest
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	if len(body.Users) == 0 {
		return httputil.FailBadRequest(c, "users required")
	}

	results := make([]map[string]any, 0, len(body.Users))
	inserted := 0
	duplicates := 0
	errorCount := 0
	for _, u := range body.Users {
		err := ctrl.service.InviteUser(ctx, tenantId, orgId, callerUserId, u.UserId, u.Role)

		result := map[string]any{
			"userId": u.UserId,
			"role":   u.Role,
		}

		if err != nil {
			result["success"] = false
			result["error"] = err.Error()
			// ✅ แยก duplicate vs error
			if err == authzsvc.ErrInviteAlreadyMember {
				duplicates++
			} else {
				errorCount++
			}
		} else {
			result["success"] = true
			inserted++
		}

		results = append(results, result)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": fmt.Sprintf("insert %d, remove 0, duplicate %d, error %d", inserted, duplicates, errorCount),
		"status":  true,
		"details": results,
	})
}
