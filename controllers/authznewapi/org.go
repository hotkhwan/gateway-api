// controllers/authznewapi/org.go
package authznewapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

type CreateOrgRequest struct {
	Name        string  `json:"name" example:"Aliza Corp"`
	Description *string `json:"description,omitempty" example:"Main tenant organization"`
}

type UpdateOrgRequest struct {
	Name        string  `json:"name" example:"Aliza Corp Updated"`
	Description *string `json:"description,omitempty" example:"Updated description"`
}

// =========================
// LIST ORGANIZATIONS
// =========================

// ListOrgs godoc
// @Summary List organizations for current user
// @Description Return organizations that current user can access (via FGA lookup)
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Success 200 {object} gmod.OrgListResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs [get]
func ListOrgs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "Unauthorized",
			Status:  false,
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

	orgs, err := authzsvc.ListUserOrganizations(ctx, tenantId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
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

// CreateOrg godoc
// @Summary Create organization
// @Description Create new organization and bootstrap FGA tuples
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateOrgRequest true "Create Organization"
// @Success 200 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs [post]
func CreateOrg(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("authznewapi")
	ctx, span := tracer.Start(ctx, "CreateOrg")
	defer span.End()

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "Unauthorized",
			Status:  false,
		})
	}

	var body CreateOrgRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid body",
			Status:  false,
		})
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "name is required",
			Status:  false,
		})
	}

	org, err := authzsvc.BootstrapOrganization(
		ctx,
		tenantId,
		userId,
		body.Name,
		body.Description,
	)
	if err != nil {
		return c.Status(409).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageCreateResponse{
		Code:    gmod.CodeCreated,
		Message: "Organization created",
		Status:  true,
		ID:      org.OrgId,
	})
}

// =========================
// UPDATE ORGANIZATION
// =========================

// UpdateOrg godoc
// @Summary Update organization
// @Description Update organization name or description
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param body body UpdateOrgRequest true "Update Organization"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/{id} [patch]
func UpdateOrg(c *fiber.Ctx) error {
	ctx := c.UserContext()

	orgId := strings.TrimSpace(c.Params("id"))

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if orgId == "" || userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "Unauthorized",
			Status:  false,
		})
	}

	var body UpdateOrgRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid body",
			Status:  false,
		})
	}

	err := authzsvc.UpdateOrg(
		ctx,
		tenantId,
		userId,
		orgId,
		body.Name,
		body.Description,
	)

	if err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organization updated",
		Status:  true,
	})
}

// =========================
// DELETE ORGANIZATION
// =========================

// DeleteOrg godoc
// @Summary Delete organization
// @Description Delete organization (remove FGA tuples first)
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/{id} [delete]
func DeleteOrg(c *fiber.Ctx) error {
	ctx := c.UserContext()

	orgId := strings.TrimSpace(c.Params("id"))

	userId, _ := c.Locals("userId").(string)
	tenantId, _ := c.Locals("tenantId").(string)

	if orgId == "" || userId == "" || tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "Unauthorized",
			Status:  false,
		})
	}

	err := authzsvc.DeleteOrg(ctx, tenantId, userId, orgId)
	if err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organization deleted",
		Status:  true,
	})
}
