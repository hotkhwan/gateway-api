// controllers/workspaceapi/entitlement.go
package workspaceapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type WorkspaceEntitlementController struct {
	svc *entitlementsvc.EntitlementService
}

func NewWorkspaceEntitlementController(svc *entitlementsvc.EntitlementService) *WorkspaceEntitlementController {
	if svc == nil {
		panic("WorkspaceEntitlementController: svc required")
	}
	return &WorkspaceEntitlementController{svc: svc}
}

// GetEntitlement godoc
// @Summary      Get workspace runtime entitlement
// @Description  Returns the cached RuntimeEntitlement snapshot for the active workspace. Read-only.
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Workspace  header  string  true  "Active Workspace ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/entitlement [get]
func (ctrl *WorkspaceEntitlementController) GetEntitlement(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceEntitlementController.Get", "workspaceapi", "GetEntitlement")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "X-Active-Workspace header required", Status: false,
		})
	}

	ent, err := ctrl.svc.GetWorkspaceEntitlement(ctx, workspaceId)
	if err != nil {
		return httputil.FailNotFound(c, "entitlement not found — snapshot may not have been received yet")
	}

	return httputil.Ok(c, ent)
}
