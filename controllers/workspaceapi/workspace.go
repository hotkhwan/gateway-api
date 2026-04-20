// controllers/workspaceapi/workspace.go
package workspaceapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type WorkspaceController struct {
	svc       *workspacesvc.WorkspaceService
	authzClient authzgw.Client
}

func NewWorkspaceController(svc *workspacesvc.WorkspaceService, authzClient authzgw.Client) *WorkspaceController {
	if svc == nil {
		panic("WorkspaceController: svc required")
	}
	return &WorkspaceController{svc: svc, authzClient: authzClient}
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

type updateWorkspaceRequest struct {
	Name *string `json:"name,omitempty"`
}

// List godoc
// @Summary      List workspaces for current user
// @Description  Returns all workspaces the authenticated user has access to.
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces [get]
func (ctrl *WorkspaceController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceController.List", "workspaceapi", "List")
	defer end()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)
	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}

	role, _ := c.Locals("role").(string)
	isPlatformAdmin := role == "administrator"

	workspaces, err := ctrl.svc.ListForUser(ctx, tenantId, userId, ctrl.authzClient, isPlatformAdmin)
	if err != nil {
		return httputil.FailInternal(c, "list workspaces failed")
	}

	return httputil.Ok(c, fiber.Map{"items": workspaces})
}

// Create godoc
// @Summary      Create a standalone workspace
// @Description  Creates a new workspace not linked to a klynx org. Caller becomes owner.
// @Tags         Workspaces
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  createWorkspaceRequest  true  "Workspace input"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces [post]
func (ctrl *WorkspaceController) Create(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceController.Create", "workspaceapi", "Create")
	defer end()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)
	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}

	var req createWorkspaceRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "name is required", Status: false,
		})
	}

	ws, err := ctrl.svc.CreateStandalone(ctx, tenantId, req.Name, userId)
	if err != nil {
		return httputil.FailInternal(c, "create workspace failed")
	}

	return httputil.Created(c, ws, "workspace created")
}

// GetByID godoc
// @Summary      Get workspace by ID
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Workspace ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{id} [get]
func (ctrl *WorkspaceController) GetByID(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceController.GetByID", "workspaceapi", "GetByID")
	defer end()

	workspaceId := c.Params("id")
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "workspace id required")
	}

	ws, err := ctrl.svc.GetByID(ctx, workspaceId)
	if err != nil {
		return httputil.FailNotFound(c, "workspace not found")
	}

	return httputil.Ok(c, ws)
}

// Update godoc
// @Summary      Update workspace
// @Tags         Workspaces
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string                  true  "Workspace ID"
// @Param        body  body  updateWorkspaceRequest  true  "Update input"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{id} [patch]
func (ctrl *WorkspaceController) Update(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceController.Update", "workspaceapi", "Update")
	defer end()

	workspaceId := c.Params("id")
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "workspace id required")
	}

	var req updateWorkspaceRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.Name == nil || *req.Name == "" {
		return httputil.FailBadRequest(c, "name is required")
	}

	if err := ctrl.svc.UpdateName(ctx, workspaceId, *req.Name); err != nil {
		return httputil.FailInternal(c, "update workspace failed")
	}

	return httputil.MessageOK(c, "workspace updated")
}

// Delete godoc
// @Summary      Delete standalone workspace
// @Description  Deletes a standalone workspace (not klynx-provisioned). Klynx workspaces must be suspended via org deletion.
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Workspace ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{id} [delete]
func (ctrl *WorkspaceController) Delete(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceController.Delete", "workspaceapi", "Delete")
	defer end()

	workspaceId := c.Params("id")
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "workspace id required")
	}

	if err := ctrl.svc.DeleteStandalone(ctx, workspaceId); err != nil {
		return httputil.FailBadRequest(c, err.Error())
	}

	return httputil.MessageOK(c, "workspace deleted")
}
