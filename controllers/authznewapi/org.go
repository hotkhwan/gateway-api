// controllers/authznewapi/org.go
package authznewapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Summary List organizations for current user
// @Tags AuthzNew
// @Security BearerAuth
// @Produce json
func ListOrgs(c *fiber.Ctx) error {

	ctx := c.UserContext()

	userId, ok := c.Locals("userId").(string)
	if !ok || userId == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(
			gmod.UnauthorizedResponse{
				Code:    gmod.CodeUnauthorized,
				Message: "Unauthorized",
				Status:  false,
			},
		)
	}

	tenantId, ok := c.Locals("tenantId").(string)
	if !ok || tenantId == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(
			gmod.UnauthorizedResponse{
				Code:    gmod.CodeUnauthorized,
				Message: "Unauthorized",
				Status:  false,
			},
		)
	}

	// Pagination params
	page := c.QueryInt("page", 1)
	perPages := c.QueryInt("perPages", 10)

	orgs, err := authzsvc.ListUserOrganizations(ctx, tenantId, userId)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			gmod.ApiErrorResponse{
				Code:    gmod.CodeInternalError,
				Message: "Failed to fetch organizations",
				Status:  false,
			},
		)
	}

	total := len(orgs)

	start := (page - 1) * perPages
	end := start + perPages

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paged := orgs[start:end]

	totalPages := 0
	if perPages > 0 {
		totalPages = (total + perPages - 1) / perPages
	}

	return c.JSON(gmod.OrgListResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organizations fetched successfully",
		Status:  true,
		Details: paged,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     perPages,
			TotalRecords: total,
			TotalPages:   totalPages,
			SortField:    "",
			SortOrder:    "",
		},
	})
}

// @Summary Create organization
// @Tags AuthzNew
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
func CreateOrg(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authznewapi")
	ctx, span := tracer.Start(ctx, "AuthzNew.CreateOrg")
	defer span.End()

	userId, ok := c.Locals("userId").(string)
	if !ok || strings.TrimSpace(userId) == "" {
		return fiber.ErrUnauthorized
	}

	tenantId, ok := c.Locals("tenantId").(string)
	if !ok || strings.TrimSpace(tenantId) == "" {
		return fiber.ErrUnauthorized
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.ErrBadRequest
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name required")
	}

	org, err := authzsvc.BootstrapOrganization(ctx, tenantId, userId, req.Name)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"status":  true,
		"orgId":   org.OrgId,
		"message": "organization created",
	})
}

// UpdateOrg godoc
// @Summary Update organization
// @Tags 2.authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Org ID"
// @Param body body map[string]string true "name"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Router /orgs/{id} [patch]
func UpdateOrg(c *fiber.Ctx) error {
	orgID := c.Params("id")

	var body struct {
		Name string `json:"name"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	if err := authzsvc.UpdateOrg(c.Context(), orgID, body.Name); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organization updated successfully",
		Status:  true,
	})
}

// DeleteOrg godoc
// @Summary Delete organization
// @Tags 2.authorization
// @Security BearerAuth
// @Param id path string true "Org ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Router /orgs/{id} [delete]
func DeleteOrg(c *fiber.Ctx) error {
	orgID := c.Params("id")

	if err := authzsvc.DeleteOrg(c.Context(), orgID); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "Organization deleted successfully",
		Status:  true,
	})
}
