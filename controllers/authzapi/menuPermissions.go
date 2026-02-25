// controllers/authzapi/menuPermissions.go
package authzapi

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

type MenuPermissionsProfileController struct {
	svc      *authzsvc.ProfileMenuPermissionService
	menuRepo *authzrepo.MenuListRepo
}

func NewMenuPermissionsProfileController(
	svc *authzsvc.ProfileMenuPermissionService,
	menuRepo *authzrepo.MenuListRepo,
) *MenuPermissionsProfileController {
	return &MenuPermissionsProfileController{svc: svc, menuRepo: menuRepo}
}

// ============================================================
// Request types
// ============================================================

type createMenuProfileBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      bool     `json:"status"`
	Menus       []string `json:"menus"`     // ["kcontrol","dashboard",...]
	Relations   []string `json:"relations"` // ["viewer","creator",...] default ["viewer"]
}

type updateMenuProfileBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      bool   `json:"status"`
	// nil = not provided (keep old), [] = clear/reset, [...] = set
	OrgUnits  []string `json:"orgUnits"`
	Menus     []string `json:"menus"`
	Relations []string `json:"relations"`
}

// ListMenus godoc
// @Summary      List all available menus
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/list [get]
func (ctrl *MenuPermissionsProfileController) ListMenus(c *fiber.Ctx) error {
	menus, err := ctrl.menuRepo.LoadMenuList(c.UserContext())
	if err != nil {
		if errors.Is(err, authzrepo.ErrMenuListNotFound) {
			return c.Status(404).JSON(gmod.ApiErrorResponse{Code: gmod.CodeNotFound, Message: "menu list not found", Status: false})
		}
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: "internal server error", Status: false})
	}
	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu list fetched",
		"status":  true,
		"detail":  menus,
	})
}

// Create godoc
// @Summary      Create menu permission profile
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body createMenuProfileBody true "payload"
// @Success      201 {object} gmod.SuccessDetailResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      409 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/permissions [post]
func (ctrl *MenuPermissionsProfileController) Create(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	var body createMenuProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name is required", Status: false})
	}

	profile, err := ctrl.svc.Create(c.UserContext(), authzsvc.CreateMenuProfileInput{
		TenantID:    tenantId,
		OrgID:       orgId,
		CallerID:    userId,
		Name:        body.Name,
		Description: body.Description,
		Status:      body.Status,
		MenuIDs:     body.Menus,
		Relations:   body.Relations,
	})
	if err != nil {
		return handleMenuProfileErr(c, err)
	}

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu permission profile created",
		"status":  true,
		"detail":  profile,
	})
}

// Update godoc
// @Summary      Update menu permission profile
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path string true "profileId"
// @Param        body body updateMenuProfileBody true "payload"
// @Success      200 {object} gmod.SuccessDetailResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/permissions/{id} [patch]
func (ctrl *MenuPermissionsProfileController) Update(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	var body updateMenuProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}

	updated, err := ctrl.svc.Update(c.UserContext(), authzsvc.UpdateMenuProfileInput{
		TenantID:    tenantId,
		OrgID:       orgId,
		ProfileID:   profileId,
		CallerID:    userId,
		Name:        body.Name,
		Description: body.Description,
		Status:      body.Status,
		OrgUnitIDs:  body.OrgUnits,  // nil when field absent in JSON
		MenuIDs:     body.Menus,     // nil when field absent in JSON
		Relations:   body.Relations, // nil when field absent in JSON
	})
	if err != nil {
		return handleMenuProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu permission profile updated",
		"status":  true,
		"detail":  updated,
	})
}

// Delete godoc
// @Summary      Delete menu permission profile
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "profileId"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/permissions/{id} [delete]
func (ctrl *MenuPermissionsProfileController) Delete(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	if err := ctrl.svc.Delete(c.UserContext(), tenantId, orgId, profileId, userId); err != nil {
		return handleMenuProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu permission profile deleted",
		"status":  true,
	})
}

// List godoc
// @Summary      List menu permission profiles
// @Tags         MenuPermissionProfile
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
// @Router       /api/v1/orgs/menu/permissions [get]
func (ctrl *MenuPermissionsProfileController) List(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	profiles, total, err := ctrl.svc.List(c.UserContext(), authzsvc.ListMenuProfileInput{
		TenantID:  tenantId,
		OrgID:     orgId,
		Search:    c.Query("search"),
		Page:      page,
		PerPage:   perPages,
		SortField: c.Query("sortField", "createdAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	})
	if err != nil {
		return handleMenuProfileErr(c, err)
	}

	totalPages := int((total + int64(perPages) - 1) / int64(perPages))
	if totalPages == 0 {
		totalPages = 1
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu permission profiles fetched",
		"status":  true,
		"details": profiles,
		"pagination": fiber.Map{
			"page":         page,
			"perPages":     perPages,
			"totalRecords": total,
			"totalPages":   totalPages,
		},
	})
}

// GetOne godoc
// @Summary      Get menu permission profile by ID
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "profileId"
// @Success      200 {object} gmod.SuccessDetailResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/permissions/{id} [get]
func (ctrl *MenuPermissionsProfileController) GetOne(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	profileId := c.Params("id")

	profile, err := ctrl.svc.Get(c.UserContext(), tenantId, orgId, profileId)
	if err != nil {
		return handleMenuProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "menu permission profile fetched",
		"status":  true,
		"detail":  profile,
	})
}

// ============================================================
// Error handler
// ============================================================

func handleMenuProfileErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, authzsvc.ErrForbidden):
		return c.Status(403).JSON(gmod.ApiErrorResponse{Code: gmod.CodeForbidden, Message: "forbidden", Status: false})
	case errors.Is(err, authzsvc.ErrNotFound),
		errors.Is(err, authzrepo.ErrMenuProfileNotFound):
		return c.Status(404).JSON(gmod.ApiErrorResponse{Code: gmod.CodeNotFound, Message: err.Error(), Status: false})
	case errors.Is(err, authzrepo.ErrMenuProfileNameExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{Code: gmod.CodeConflict, Message: err.Error(), Status: false})
	case errors.Is(err, authzsvc.ErrInvalidArgs):
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: err.Error(), Status: false})
	default:
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: "internal server error", Status: false})
	}
}
