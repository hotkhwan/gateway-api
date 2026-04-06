// controllers/authzapi/menuPermissions.go
package authzapi

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
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
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /orgs/menu/list [get]
func (ctrl *MenuPermissionsProfileController) ListMenus(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.ListMenus", "authzapi", "ListMenus")
	defer end()

	menus, err := ctrl.menuRepo.LoadMenuList(c)
	if err != nil {
		if errors.Is(err, authzrepo.ErrMenuListNotFound) {
			return httputil.FailNotFound(c, "menu list not found")
		}
		return httputil.FailInternal(c, "internal server error")
	}
	return httputil.Ok(c, menus, "menu list fetched")
}

// Create godoc
// @Summary      Create menu permission profile
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body createMenuProfileBody true "payload"
// @Success      201 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      409 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /orgs/menu/permissions [post]
func (ctrl *MenuPermissionsProfileController) Create(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.Create", "authzapi", "Create")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	var body createMenuProfileBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name is required")
	}

	profile, err := ctrl.svc.Create(c, authzsvc.CreateMenuProfileInput{
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

	return httputil.Created(c, profile, "menu permission profile created")
}

// Update godoc
// @Summary      Update menu permission profile
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path string true "profileId"
// @Param        body body updateMenuProfileBody true "payload"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /orgs/menu/permissions/{id} [patch]
func (ctrl *MenuPermissionsProfileController) Update(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.Update", "authzapi", "Update")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	var body updateMenuProfileBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	updated, err := ctrl.svc.Update(c, authzsvc.UpdateMenuProfileInput{
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

	return httputil.Ok(c, updated, "menu permission profile updated")
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
// @Router       /orgs/menu/permissions/{id} [delete]
func (ctrl *MenuPermissionsProfileController) Delete(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.Delete", "authzapi", "Delete")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	if err := ctrl.svc.Delete(c, tenantId, orgId, profileId, userId); err != nil {
		return handleMenuProfileErr(c, err)
	}

	return httputil.MessageOK(c, "menu permission profile deleted")
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
// @Router       /orgs/menu/permissions [get]
func (ctrl *MenuPermissionsProfileController) List(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.List", "authzapi", "List")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	profiles, total, err := ctrl.svc.List(c, authzsvc.ListMenuProfileInput{
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

	return c.JSON(gmod.PaginatedResponse{
		Code:    gmod.CodeSuccess,
		Message: "menu permission profiles fetched",
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
// @Summary      Get menu permission profile by ID
// @Tags         MenuPermissionProfile
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "profileId"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /orgs/menu/permissions/{id} [get]
func (ctrl *MenuPermissionsProfileController) GetOne(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "MenuPermissionsProfileController.GetOne", "authzapi", "GetOne")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	profileId := c.Params("id")

	profile, err := ctrl.svc.Get(c, tenantId, orgId, profileId)
	if err != nil {
		return handleMenuProfileErr(c, err)
	}

	return httputil.Ok(c, profile, "menu permission profile fetched")
}

// ============================================================
// Error handler
// ============================================================

func handleMenuProfileErr(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, authzsvc.ErrForbidden):
		return httputil.FailForbidden(c, "forbidden")
	case errors.Is(err, authzsvc.ErrNotFound),
		errors.Is(err, authzrepo.ErrMenuProfileNotFound):
		return httputil.FailNotFound(c, err.Error())
	case errors.Is(err, authzrepo.ErrMenuProfileNameExists):
		return httputil.FailConflict(c, err.Error())
	case errors.Is(err, authzsvc.ErrInvalidArgs):
		return httputil.FailBadRequest(c, err.Error())
	default:
		return httputil.FailInternal(c, "internal server error")
	}
}
