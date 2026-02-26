// controllers/authzapi/org.go
package authzapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"go.opentelemetry.io/otel"
)

type OrganizationController struct {
	service *authzsvc.OrganizationService
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

func NewOrganizationController(svc *authzsvc.OrganizationService) *OrganizationController {
	if svc == nil {
		panic("organizationService required")
	}
	return &OrganizationController{service: svc}
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
// @Router /api/v1/orgs [get]
func (ctrl *OrganizationController) List(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.List")
	defer span.End()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)
	// Use user's stored activeOrgId from JWT attributes, fallback to X-Active-Org header
	userActiveOrgId, _ := c.Locals("userActiveOrgId").(string)
	activeOrg, _ := c.Locals("activeOrg").(string)
	activeOrgId := userActiveOrgId
	if activeOrgId == "" {
		activeOrgId = activeOrg
	}

	if userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("perPage", 10)

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
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	total := len(orgs)
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paged := orgs[start:end]
	totalPages := (total + perPage - 1) / perPage

	return c.JSON(gmod.OrgListResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organizations fetched successfully",
		Status:  true,
		Details: paged,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     perPage,
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
// @Router /api/v1/orgs [post]
func (ctrl *OrganizationController) Create(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.Create")
	defer span.End()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	var body CreateOrgRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
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
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
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
// @Summary Get ingest config for an organization (admin only)
// @Description Returns ingest endpoint, masked secret, and rate limit config
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/{id}/ingest [get]
func (ctrl *OrganizationController) GetIngestConfig(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.GetIngestConfig")
	defer span.End()

	orgId := strings.TrimSpace(c.Locals("activeOrg").(string))
	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	cfg, err := ctrl.service.GetIngestConfig(ctx, tenantId, userId, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "ingest config fetched",
		Data:    cfg,
	})
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
// @Router /api/v1/orgs/{id}/ingest/rotateSecret [post]
func (ctrl *OrganizationController) RotateIngestSecret(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.RotateIngestSecret")
	defer span.End()

	orgId := strings.TrimSpace(c.Locals("activeOrg").(string))
	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	cfg, err := ctrl.service.RotateIngestSecret(ctx, tenantId, userId, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessDataResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "ingest secret rotated",
		Data:    cfg,
	})
}

// =========================
// UPDATE ORGANIZATION
// =========================

// Update godoc
// @Summary Update organization
// @Tags Authorization
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [patch]
func (ctrl *OrganizationController) Update(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.Update")
	defer span.End()

	orgId := strings.TrimSpace(c.Params("id"))

	var body UpdateOrgRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
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
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "organization updated",
	})
}

// =========================
// DELETE ORGANIZATION
// =========================

// Delete godoc
// @Summary Delete organization
// @Tags Authorization
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [delete]
func (ctrl *OrganizationController) Delete(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "Organization.Delete")
	defer span.End()

	orgId := strings.TrimSpace(c.Params("id"))

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	err := ctrl.service.Delete(ctx, tenantId, userId, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "organization deleted",
	})
}
