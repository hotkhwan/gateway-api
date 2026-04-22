// controllers/workspaceapi/entitlement.go
package workspaceapi

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// entitlementReader is the minimal surface this controller uses, kept as an
// interface so tests can exercise the 200/400/500 paths without standing up
// a Redis client.
type entitlementReader interface {
	GetForWorkspace(ctx context.Context, workspaceId, tenantId string) (*entitlementsvc.RuntimeEntitlement, error)
}

type WorkspaceEntitlementController struct {
	svc entitlementReader
}

func NewWorkspaceEntitlementController(svc *entitlementsvc.EntitlementService) *WorkspaceEntitlementController {
	if svc == nil {
		panic("WorkspaceEntitlementController: svc required")
	}
	return &WorkspaceEntitlementController{svc: svc}
}

// GetEntitlement godoc
// @Summary      Get workspace runtime entitlement
// @Description  Returns the RuntimeEntitlement for the active workspace. On cache miss the service synthesizes a product-neutral snapshot from the deployment profile catalog (overlaid with the tenant's local subscription in saasPublic), so the caller always receives a usable entitlement without having to translate "not cached yet" into a user-visible error.
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Workspace  header  string  true  "Active Workspace ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/entitlement [get]
func (ctrl *WorkspaceEntitlementController) GetEntitlement(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceEntitlementController.Get", "workspaceapi", "GetEntitlement")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "X-Active-Workspace header required", Status: false,
		})
	}
	tenantId, _ := c.Locals("tenantId").(string)

	ent, err := ctrl.svc.GetForWorkspace(ctx, workspaceId, tenantId)
	if err != nil {
		// Cache miss is not an error path anymore — it is synthesized inside
		// the service. Only real Redis decode/read failures or subscription
		// overlay lookup failures reach this branch, so surface them as 500.
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("entitlement lookup failed")
		return httputil.FailInternal(c, "failed to load entitlement")
	}

	return httputil.Ok(c, ent)
}
