// controllers/authzapi/resourcePermissions.go
package authzapi

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

type ResourcePermissionsProfileController struct {
	svc *authzsvc.PermissionProfileService
}

func NewResourcePermissionsProfileController(svc *authzsvc.PermissionProfileService) *ResourcePermissionsProfileController {
	return &ResourcePermissionsProfileController{svc: svc}
}

// ============================================================
// Request types
// ============================================================

type createPermProfileBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      bool     `json:"status"`
	Relations   []string `json:"relations"`
}

type updatePermProfileBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      bool     `json:"status"`
	Relations   []string `json:"relations"`
	// flat string IDs — nil = not provided (keep old), [] = clear, ["id"] = set
	OrgUnits       []string `json:"orgUnits"`
	ResourceGroups []string `json:"resourceGroups"`
}

// Create godoc
// @Summary      Create resource permission profile
// @Tags         ResourcePermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body createPermProfileBody true "payload"
// @Success      201 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      409 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/permissions [post]
func (ctrl *ResourcePermissionsProfileController) Create(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	var body createPermProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name is required", Status: false})
	}
	if len(body.Relations) == 0 {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "relations is required", Status: false})
	}

	profile, err := ctrl.svc.Create(c.UserContext(), authzsvc.CreatePermProfileInput{
		TenantID:    tenantId,
		OrgID:       orgId,
		CallerID:    userId,
		Name:        body.Name,
		Description: body.Description,
		Status:      body.Status,
		Relations:   body.Relations,
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile created",
		"status":  true,
		"detail":  profile,
	})
}

// Update godoc
// @Summary      Update resource permission profile
// @Tags         ResourcePermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path string true "profileId"
// @Param        body body updatePermProfileBody true "payload"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/permissions/{id} [patch]
func (ctrl *ResourcePermissionsProfileController) Update(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	var body updatePermProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}

	updated, err := ctrl.svc.Update(c.UserContext(), authzsvc.UpdatePermProfileInput{
		TenantID:         tenantId,
		OrgID:            orgId,
		ProfileID:        profileId,
		CallerID:         userId,
		Name:             body.Name,
		Description:      body.Description,
		Status:           body.Status,
		Relations:        body.Relations,
		OrgUnitIDs:       body.OrgUnits,       // nil when field absent in JSON
		ResourceGroupIDs: body.ResourceGroups, // nil when field absent in JSON
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile updated",
		"status":  true,
		"detail":  updated,
	})
}

// Delete godoc
// @Summary      Delete resource permission profile
// @Tags         ResourcePermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "profileId"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/permissions/{id} [delete]
func (ctrl *ResourcePermissionsProfileController) Delete(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	if err := ctrl.svc.Delete(c.UserContext(), tenantId, orgId, profileId, userId); err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile deleted",
		"status":  true,
	})
}

// List godoc
// @Summary      List resource permission profiles
// @Tags         ResourcePermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        page      query int    false "page (default 1)"
// @Param        perPages  query int    false "items per page (default 10)"
// @Param        search    query string false "search keyword"
// @Param        sortField query string false "sort field (default createdAt)"
// @Param        sortOrder query string false "sort order: asc|desc (default desc)"
// @Success      200 {object} gmod.PaginatedResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/permissions [get]
func (ctrl *ResourcePermissionsProfileController) List(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	profiles, total, err := ctrl.svc.List(c.UserContext(), authzsvc.ListPermProfileInput{
		TenantID:  tenantId,
		OrgID:     orgId,
		Search:    c.Query("search"),
		Page:      page,
		PerPage:   perPages,
		SortField: c.Query("sortField", "createdAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	totalPages := int((total + int64(perPages) - 1) / int64(perPages))
	if totalPages == 0 {
		totalPages = 1
	}

	return c.JSON(gmod.PaginatedResponse{
		Code:    gmod.CodeSuccess,
		Message: "permission profiles fetched",
		Status:  true,
		Details: profiles,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     perPages,
			TotalRecords: int(total),
			TotalPages:   totalPages,
		},
	})
}

// GetOne godoc
// @Summary      Get resource permission profile by ID
// @Tags         ResourcePermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "profileId"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/permissions/{id} [get]
func (ctrl *ResourcePermissionsProfileController) GetOne(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	profileId := c.Params("id")

	profile, err := ctrl.svc.Get(c.UserContext(), tenantId, orgId, profileId)
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile fetched",
		"status":  true,
		"detail":  profile,
	})
}

// ============================================================
// Error handler
// ============================================================

func handlePermProfileErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, authzsvc.ErrForbidden):
		return c.Status(403).JSON(gmod.ApiErrorResponse{Code: gmod.CodeForbidden, Message: "forbidden", Status: false})
	case errors.Is(err, authzsvc.ErrNotFound),
		errors.Is(err, authzrepo.ErrPermProfileNotFound):
		return c.Status(404).JSON(gmod.ApiErrorResponse{Code: gmod.CodeNotFound, Message: err.Error(), Status: false})
	case errors.Is(err, authzrepo.ErrPermProfileNameExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{Code: gmod.CodeConflict, Message: err.Error(), Status: false})
	case errors.Is(err, authzsvc.ErrInvalidArgs):
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: err.Error(), Status: false})
	default:
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: "internal server error", Status: false})
	}
}
