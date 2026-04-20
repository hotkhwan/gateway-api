// controllers/workspaceapi/member.go
package workspaceapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type WorkspaceMemberController struct {
	svc *workspacesvc.WorkspaceMemberService
}

func NewWorkspaceMemberController(svc *workspacesvc.WorkspaceMemberService) *WorkspaceMemberController {
	if svc == nil {
		panic("WorkspaceMemberController: svc required")
	}
	return &WorkspaceMemberController{svc: svc}
}

type inviteMemberRequest struct {
	UserID string                      `json:"userId"`
	Role   workspacemod.WorkspaceMemberRole `json:"role"`
}

type removeMemberRequest struct {
	UserIDs []string `json:"userIds"`
}

type changeRoleRequest struct {
	Role workspacemod.WorkspaceMemberRole `json:"role"`
}

// ListMembers godoc
// @Summary      List workspace members
// @Tags         Workspaces
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Workspace  header  string  true  "Active Workspace ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/members [get]
func (ctrl *WorkspaceMemberController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceMemberController.List", "workspaceapi", "ListMembers")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "X-Active-Workspace header required", Status: false,
		})
	}

	members, err := ctrl.svc.ListMembers(ctx, tenantId, workspaceId)
	if err != nil {
		return httputil.FailInternal(c, "list members failed")
	}

	return httputil.Ok(c, fiber.Map{"items": members})
}

// Invite godoc
// @Summary      Invite a user to workspace
// @Tags         Workspaces
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Workspace  header  string              true  "Active Workspace ID"
// @Param        body                body    inviteMemberRequest true  "Invite input"
// @Success      201  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/members/invite [post]
func (ctrl *WorkspaceMemberController) Invite(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceMemberController.Invite", "workspaceapi", "InviteMember")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "X-Active-Workspace header required")
	}

	var req inviteMemberRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.UserID == "" {
		return httputil.FailBadRequest(c, "userId is required")
	}
	if !workspacemod.IsValidRole(req.Role) {
		return httputil.FailBadRequest(c, "role must be one of: owner, admin, operator, viewer")
	}

	if err := ctrl.svc.InviteMember(ctx, workspaceId, req.UserID, req.Role); err != nil {
		return httputil.FailInternal(c, "invite member failed")
	}

	return httputil.MessageOK(c, "member invited")
}

// Remove godoc
// @Summary      Remove members from workspace
// @Tags         Workspaces
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Workspace  header  string              true  "Active Workspace ID"
// @Param        body                body    removeMemberRequest true  "Remove input"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/members/remove [patch]
func (ctrl *WorkspaceMemberController) Remove(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceMemberController.Remove", "workspaceapi", "RemoveMember")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "X-Active-Workspace header required")
	}

	var req removeMemberRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if len(req.UserIDs) == 0 {
		return httputil.FailBadRequest(c, "userIds is required")
	}

	var failed []string
	for _, uid := range req.UserIDs {
		if err := ctrl.svc.RemoveMember(ctx, workspaceId, uid); err != nil {
			failed = append(failed, uid)
		}
	}
	if len(failed) > 0 {
		return httputil.Ok(c, fiber.Map{
			"removed": len(req.UserIDs) - len(failed),
			"failed":  failed,
		}, "partial success")
	}

	return httputil.MessageOK(c, "members removed")
}

// ChangeRole godoc
// @Summary      Change a member's role
// @Tags         Workspaces
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Workspace  header  string            true  "Active Workspace ID"
// @Param        userId              path    string            true  "User ID"
// @Param        body                body    changeRoleRequest true  "Role input"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/members/{userId}/role [patch]
func (ctrl *WorkspaceMemberController) ChangeRole(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.workspaceapi", "WorkspaceMemberController.ChangeRole", "workspaceapi", "ChangeRole")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId := c.Params("userId")
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "X-Active-Workspace header required")
	}
	if userId == "" {
		return httputil.FailBadRequest(c, "userId path param required")
	}

	var req changeRoleRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if !workspacemod.IsValidRole(req.Role) {
		return httputil.FailBadRequest(c, "role must be one of: owner, admin, operator, viewer")
	}

	if err := ctrl.svc.ChangeRole(ctx, workspaceId, userId, req.Role); err != nil {
		return httputil.FailInternal(c, "change role failed")
	}

	return httputil.MessageOK(c, "role updated")
}
