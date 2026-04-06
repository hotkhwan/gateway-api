// controllers/authzapi/resourcePermissions.go
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
func (ctrl *ResourcePermissionsProfileController) Create(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "ResourcePermissionsProfileController.Create", "authzapi", "Create")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	var body createPermProfileBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name is required")
	}
	if len(body.Relations) == 0 {
		return httputil.FailBadRequest(c, "relations is required")
	}

	profile, err := ctrl.svc.Create(c, authzsvc.CreatePermProfileInput{
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

	return httputil.Created(c, profile, "permission profile created")
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
func (ctrl *ResourcePermissionsProfileController) Update(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "ResourcePermissionsProfileController.Update", "authzapi", "Update")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	var body updatePermProfileBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	updated, err := ctrl.svc.Update(c, authzsvc.UpdatePermProfileInput{
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

	return httputil.Ok(c, updated, "permission profile updated")
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
func (ctrl *ResourcePermissionsProfileController) Delete(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "ResourcePermissionsProfileController.Delete", "authzapi", "Delete")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	if err := ctrl.svc.Delete(c, tenantId, orgId, profileId, userId); err != nil {
		return handlePermProfileErr(c, err)
	}

	return httputil.MessageOK(c, "permission profile deleted")
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
func (ctrl *ResourcePermissionsProfileController) List(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "ResourcePermissionsProfileController.List", "authzapi", "List")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	profiles, total, err := ctrl.svc.List(c, authzsvc.ListPermProfileInput{
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
func (ctrl *ResourcePermissionsProfileController) GetOne(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "ResourcePermissionsProfileController.GetOne", "authzapi", "GetOne")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	profileId := c.Params("id")

	profile, err := ctrl.svc.Get(c, tenantId, orgId, profileId)
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return httputil.Ok(c, profile, "permission profile fetched")
}

// ============================================================
// Error handler
// ============================================================

func handlePermProfileErr(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, authzsvc.ErrForbidden):
		return httputil.FailForbidden(c, "forbidden")
	case errors.Is(err, authzsvc.ErrNotFound),
		errors.Is(err, authzrepo.ErrPermProfileNotFound):
		return httputil.FailNotFound(c, err.Error())
	case errors.Is(err, authzrepo.ErrPermProfileNameExists):
		return httputil.FailConflict(c, err.Error())
	case errors.Is(err, authzsvc.ErrInvalidArgs):
		return httputil.FailBadRequest(c, err.Error())
	default:
		return httputil.FailInternal(c, "internal server error")
	}
}
