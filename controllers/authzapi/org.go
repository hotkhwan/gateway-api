// controllers/authzapi/org.go
package authzapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type OrganizationController struct {
	service      *authzsvc.OrganizationService
	workspaceSvc *workspacesvc.WorkspaceService
}

type CreateOrgRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateOrgRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IsActive    *bool   `json:"isActive,omitempty"`
}

// Owner promotion request/response
type PromoteToOwnerRequest struct {
	UserId string `json:"userId"`
}

type DemoteFromOwnerRequest struct {
	NewRole string `json:"newRole"` // "member" or "admin"
}

func NewOrganizationController(svc *authzsvc.OrganizationService, workspaceSvc *workspacesvc.WorkspaceService) *OrganizationController {
	if svc == nil {
		panic("organizationService required")
	}
	return &OrganizationController{service: svc, workspaceSvc: workspaceSvc}
}

// =========================
// LIST ORGANIZATIONS
// =========================

// List godoc
// @Summary List organizations for current user
// @Description Return organizations that current user can access (via FGA lookup)
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Success 200 {object} gmod.OrgListResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /orgs [get]
func (ctrl *OrganizationController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.List", "authzapi", "List")
	defer end()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)
	userActiveOrgId, _ := c.Locals("userActiveOrgId").(string)
	activeWorkspace, _ := c.Locals("activeWorkspace").(string)
	activeOrgId := userActiveOrgId
	if activeOrgId == "" {
		activeOrgId = activeWorkspace
	}

	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}

	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPage", 10)

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}
	if perPage > 100 {
		perPage = 100
	}

	orgs, err := ctrl.service.List(ctx, tenantId, userId, activeOrgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	total := len(orgs)
	start := (page - 1) * perPage
	end2 := start + perPage
	if start > total {
		start = total
	}
	if end2 > total {
		end2 = total
	}

	paged := orgs[start:end2]
	totalPages := (total + perPage - 1) / perPage

	return c.JSON(gmod.OrgListResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organizations fetched successfully",
		Status:  true,
		Details: paged,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPage:      perPage,
			TotalRecords: total,
			TotalPages:   totalPages,
		},
	})
}

// =========================
// CREATE ORGANIZATION
// =========================

// Create godoc
// @Summary Create organization
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /orgs [post]
func (ctrl *OrganizationController) Create(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.Create", "authzapi", "Create")
	defer end()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	var body CreateOrgRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	orgId, err := ctrl.service.Create(
		ctx,
		tenantId,
		userId,
		body.Name,
		body.Description,
	)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	return c.JSON(gmod.SuccessMessageCreateResponse{
		Code:    gmod.CodeCreated,
		Status:  true,
		Message: "organization created",
		ID:      orgId,
	})
}

// =========================
// GET INGEST CONFIG
// =========================

// GetIngestConfig godoc
// @Summary Get ingest config for the active workspace
// @Description Returns ingest endpoint, masked secret, and rate limit config. Auto-provisions ingest key on first call.
// @Tags Ingest
// @Security BearerAuth
// @Produce json
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 404 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /ingest [get]
func (ctrl *OrganizationController) GetIngestConfig(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.GetIngestConfig", "authzapi", "GetIngestConfig")
	defer end()

	workspaceId := strings.TrimSpace(c.Locals("activeWorkspace").(string))
	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "activeWorkspace required")
	}

	if ctrl.workspaceSvc == nil {
		return httputil.FailInternal(c, "workspace service not configured")
	}

	cfg, err := ctrl.workspaceSvc.GetIngestConfig(ctx, workspaceId)
	if err != nil {
		return httputil.FailInternal(c, "get ingest config failed")
	}

	return httputil.Ok(c, cfg, "ingest config fetched")
}

// =========================
// ROTATE INGEST SECRET
// =========================

// RotateIngestSecret godoc
// @Summary Rotate ingest secret for an organization (admin only)
// @Description Generates a new HMAC signing key and returns the masked version
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Router /orgs/{id}/ingest/rotateSecret [post]
// RotateIngestSecret godoc
// @Summary Rotate ingest secret for the active workspace
// @Description Generates a new HMAC signing key and returns the masked version
// @Tags Ingest
// @Security BearerAuth
// @Produce json
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 404 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /ingest/rotateSecret [post]
func (ctrl *OrganizationController) RotateIngestSecret(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.RotateIngestSecret", "authzapi", "RotateIngestSecret")
	defer end()

	workspaceId := strings.TrimSpace(c.Locals("activeWorkspace").(string))
	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}
	if workspaceId == "" {
		return httputil.FailBadRequest(c, "activeWorkspace required")
	}

	if ctrl.workspaceSvc == nil {
		return httputil.FailInternal(c, "workspace service not configured")
	}

	cfg, err := ctrl.workspaceSvc.RotateIngestSecret(ctx, workspaceId)
	if err != nil {
		return httputil.FailInternal(c, "rotate ingest secret failed")
	}

	return httputil.Ok(c, cfg, "ingest secret rotated")
}

// =========================
// UPDATE ORGANIZATION
// =========================

// Update godoc
// @Summary Update organization
// @Tags Authorization
// @Security BearerAuth
// @Router /orgs/{id} [patch]
func (ctrl *OrganizationController) Update(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.Update", "authzapi", "Update")
	defer end()

	orgId := strings.TrimSpace(c.Params("id"))

	var body UpdateOrgRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	err := ctrl.service.Update(
		ctx,
		tenantId,
		userId,
		orgId,
		body.Name,
		body.Description,
		body.IsActive,
	)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	return httputil.MessageOK(c, "organization updated")
}

// =========================
// DELETE ORGANIZATION
// =========================

// Delete godoc
// @Summary Delete organization
// @Tags Authorization
// @Security BearerAuth
// @Router /orgs/{id} [delete]
func (ctrl *OrganizationController) Delete(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.Delete", "authzapi", "Delete")
	defer end()

	orgId := strings.TrimSpace(c.Params("id"))

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	err := ctrl.service.Delete(ctx, tenantId, userId, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	return httputil.MessageOK(c, "organization deleted")
}

// =========================
// OWNER MANAGEMENT
// =========================

// PromoteToOwner godoc
// @Summary Promote a member to owner
// @Description Promote a member to owner with invariant checks + race protection
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "Organization ID"
// @Param userId path string true "User ID to promote"
// @Success 200 {object} gmod.PromoteToOwnerResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Router /orgs/{id}/owners/{userId} [post]
func (ctrl *OrganizationController) PromoteToOwner(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.PromoteToOwner", "authzapi", "PromoteToOwner")
	defer end()

	orgId := strings.TrimSpace(c.Params("id"))
	userId := strings.TrimSpace(c.Params("userId"))
	callerUserId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || callerUserId == "" || tenantId == "" {
		return httputil.FailBadRequest(c, "missing required parameters")
	}

	err := ctrl.service.PromoteUserToOwner(ctx, tenantId, orgId, callerUserId, userId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	var result gmod.PromoteToOwnerResponse
	result.Code = gmod.CodeSuccess
	result.Status = true
	result.Message = "user promoted to owner"
	result.Details.WorkspaceId = orgId
	result.Details.UserId = userId
	result.Details.Role = "owner"
	result.Details.PromotedAt = time.Now().Unix()

	return c.JSON(result)
}

// DemoteFromOwner godoc
// @Summary Demote an owner to member or admin
// @Description Demote an owner to member or admin with invariant checks + race protection
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "Organization ID"
// @Param userId path string true "User ID to demote"
// @Param newRole query string false "New role (member or admin)"
// @Success 200 {object} gmod.DemoteFromOwnerResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Router /orgs/{id}/owners/{userId} [delete]
func (ctrl *OrganizationController) DemoteFromOwner(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.DemoteFromOwner", "authzapi", "DemoteFromOwner")
	defer end()

	orgId := strings.TrimSpace(c.Params("id"))
	userId := strings.TrimSpace(c.Params("userId"))
	newRole := strings.ToLower(strings.TrimSpace(c.Query("newRole", "member")))
	callerUserId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || callerUserId == "" || tenantId == "" {
		return httputil.FailBadRequest(c, "missing required parameters")
	}

	err := ctrl.service.DemoteUserFromOwner(ctx, tenantId, orgId, callerUserId, userId, newRole)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	var result gmod.DemoteFromOwnerResponse
	result.Code = gmod.CodeSuccess
	result.Status = true
	result.Message = "user demoted from owner"
	result.Details.WorkspaceId = orgId
	result.Details.UserId = userId
	result.Details.PreviousRole = "owner"
	result.Details.NewRole = newRole
	result.Details.DemotedAt = time.Now().Unix()

	return c.JSON(result)
}

// =========================
// TRANSFER BILLING OWNERSHIP
// =========================

// TransferBillingOwnership godoc
// @Summary Transfer billing ownership to another user
// @Description Transfer billing ownership - only current billingOwnerId or owners can transfer
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param body body gmod.TransferBillingOwnershipRequest true "Transfer request"
// @Success 200 {object} gmod.TransferBillingOwnershipResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Router /orgs/{id}/transfer-billing-ownership [post]
func (ctrl *OrganizationController) TransferBillingOwnership(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.TransferBillingOwnership", "authzapi", "TransferBillingOwnership")
	defer end()

	orgId := strings.TrimSpace(c.Params("id"))
	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	var body struct {
		NewBillingOwnerId string `json:"newBillingOwnerId"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	newBillingOwnerId := strings.TrimSpace(body.NewBillingOwnerId)
	if newBillingOwnerId == "" {
		return httputil.FailBadRequest(c, "newBillingOwnerId is required")
	}

	if userId == "" || tenantId == "" {
		return httputil.FailUnauthorized(c, "Unauthorized")
	}

	// Get org to find previous billing owner
	org, err := ctrl.service.GetOrganizationByOrgId(ctx, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}
	oldBillingOwnerId := org.BillingOwnerId

	err = ctrl.service.TransferBillingOwnership(ctx, tenantId, orgId, userId, newBillingOwnerId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return httputil.Fail(c, status, code, err.Error())
	}

	var result gmod.TransferBillingOwnershipResponse
	result.Code = gmod.CodeSuccess
	result.Status = true
	result.Message = "billing ownership transferred"
	result.Details.WorkspaceId = orgId
	result.Details.PreviousOwnerId = oldBillingOwnerId
	result.Details.NewOwnerId = newBillingOwnerId
	result.Details.TransferredAt = time.Now().Unix()

	return c.JSON(result)
}
