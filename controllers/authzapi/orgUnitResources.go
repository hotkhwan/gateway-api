// controllers/authzapi/orgUnitResources.go
package authzapi

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type OrgUnitResourcesController struct {
	groupSvc *devicesvc.ResourceGroupService
}

func NewOrgUnitResourcesController(groupSvc *devicesvc.ResourceGroupService) *OrgUnitResourcesController {
	return &OrgUnitResourcesController{groupSvc: groupSvc}
}

// ============================================================
// Request / Response types
// ============================================================

type ouResourceGroupItem struct {
	ID string `json:"id"`
}

type ouResourceGroupsBody struct {
	Relation       string                `json:"relation"` // viewer | editor | deleter
	ResourceGroups []ouResourceGroupItem `json:"resourceGroups"`
}

type ouResourceBulkResponse struct {
	Code    string                        `json:"code"`
	Message string                        `json:"message"`
	Status  bool                          `json:"status"`
	Details []devicesvc.OUGroupBulkResult `json:"details"`
}

// ============================================================
// GET /orgs/units/:unitId/resources
// @Summary      List resourceGroups assigned to OrgUnit
// @Tags         OrgUnit
// @Security     BearerAuth
// @Produce      json
// @Param        unitId    path   string  true   "orgUnit id"
// @Param        search    query  string  false  "search by name"
// @Param        page      query  int     false  "page"      default(1)
// @Param        perPages  query  int     false  "per page"  default(10)
// @Param        sortField query  string  false  "sort field"
// @Param        sortOrder query  string  false  "asc|desc"
// @Success      200  {object}  map[string]interface{}
// @Router       /api/v1/orgs/units/{unitId}/resources [get]
// ============================================================

func (ctrl *OrgUnitResourcesController) ListResourceGroups(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrgUnitResourcesController.ListResourceGroups", "authzapi", "ListResourceGroups")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	unitId := c.Params("unitId")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	result, err := ctrl.groupSvc.ListResourceGroupsInOU(
		c,
		tenantId, orgId, unitId,
		devicesvc.ListInGroupParams{
			Search:    c.Query("search"),
			Page:      page,
			PerPage:   perPages,
			SortField: c.Query("sortField", "createdAt"),
			SortOrder: c.Query("sortOrder", "desc"),
		},
	)
	if err != nil {
		return handleOUResourceErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "resource groups fetched successfully",
		"status":  true,
		"details": result.Items,
		"pagination": fiber.Map{
			"page":         result.Page,
			"perPages":     result.PerPage,
			"totalRecords": result.TotalRecords,
			"totalPages":   result.TotalPages,
		},
	})
}

func handleOUResourceErr(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, devicesvc.ErrForbidden):
		return httputil.FailForbidden(c, "forbidden")
	case errors.Is(err, devicesvc.ErrInvalidArgs):
		return httputil.FailBadRequest(c, err.Error())
	case errors.Is(err, devicesvc.ErrPermifySyncFailed):
		return httputil.Fail(c, 502, "PERMIFY_SYNC_FAILED", "authorization sync failed")
	case errors.Is(err, devicerepo.ErrNotFound):
		return httputil.FailNotFound(c, "not found")
	default:
		return httputil.FailInternal(c, "internal server error")
	}
}

// ============================================================
// POST /orgs/units/:unitId/resources
// @Summary      Bulk assign resourceGroups to OrgUnit
// @Tags         OrgUnit
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        unitId  path      string                true  "orgUnit id"
// @Param        body    body      ouResourceGroupsBody  true  "payload"
// @Success      200     {object}  ouResourceBulkResponse
// @Failure      400     {object}  gmod.ApiErrorResponse
// @Failure      403     {object}  gmod.ApiErrorResponse
// @Router       /api/v1/orgs/units/{unitId}/resources [post]
// ============================================================

func (ctrl *OrgUnitResourcesController) AssignResourceGroups(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrgUnitResourcesController.AssignResourceGroups", "authzapi", "AssignResourceGroups")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	unitId := c.Params("unitId")

	var body ouResourceGroupsBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Relation == "" {
		return httputil.FailBadRequest(c, "relation is required")
	}
	if len(body.ResourceGroups) == 0 {
		return httputil.FailBadRequest(c, "resourceGroups is required")
	}

	items := make([]devicesvc.OUGroupItem, 0, len(body.ResourceGroups))
	for _, rg := range body.ResourceGroups {
		if rg.ID != "" {
			items = append(items, devicesvc.OUGroupItem{GroupID: rg.ID})
		}
	}
	if len(items) == 0 {
		return httputil.FailBadRequest(c, "no valid resourceGroup id provided")
	}

	results, inserted, duplicates, err := ctrl.groupSvc.BulkAssignGroupsToOU(
		c,
		tenantId, orgId, unitId, body.Relation, userId,
		items,
	)
	if err != nil {
		return handleOUResourceErr(c, err)
	}

	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "already assigned to this unit" && r.Error != "" {
			errorCount++
		}
	}

	return c.JSON(ouResourceBulkResponse{
		Code:    string(gmod.CodeSuccess),
		Message: fmt.Sprintf("insert %d, duplicate %d, error %d", inserted, duplicates, errorCount),
		Status:  true,
		Details: results,
	})
}

// ============================================================
// DELETE /orgs/units/:unitId/resources
// @Summary      Bulk remove resourceGroups from OrgUnit
// @Tags         OrgUnit
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        unitId  path      string                true  "orgUnit id"
// @Param        body    body      ouResourceGroupsBody  true  "payload"
// @Success      200     {object}  ouResourceBulkResponse
// @Failure      400     {object}  gmod.ApiErrorResponse
// @Failure      403     {object}  gmod.ApiErrorResponse
// @Router       /api/v1/orgs/units/{unitId}/resources [delete]
// ============================================================

func (ctrl *OrgUnitResourcesController) RemoveResourceGroups(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrgUnitResourcesController.RemoveResourceGroups", "authzapi", "RemoveResourceGroups")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	unitId := c.Params("unitId")

	var body ouResourceGroupsBody
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Relation == "" {
		return httputil.FailBadRequest(c, "relation is required")
	}
	if len(body.ResourceGroups) == 0 {
		return httputil.FailBadRequest(c, "resourceGroups is required")
	}

	items := make([]devicesvc.OUGroupItem, 0, len(body.ResourceGroups))
	for _, rg := range body.ResourceGroups {
		if rg.ID != "" {
			items = append(items, devicesvc.OUGroupItem{GroupID: rg.ID})
		}
	}
	if len(items) == 0 {
		return httputil.FailBadRequest(c, "no valid resourceGroup id provided")
	}

	results, removed, err := ctrl.groupSvc.BulkRemoveGroupsFromOU(
		c,
		tenantId, orgId, unitId, body.Relation, userId,
		items,
	)
	if err != nil {
		return handleOUResourceErr(c, err)
	}

	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "resourceGroup not assigned to this unit" && r.Error != "" {
			errorCount++
		}
	}

	return c.JSON(ouResourceBulkResponse{
		Code:    string(gmod.CodeSuccess),
		Message: fmt.Sprintf("remove %d, error %d", removed, errorCount),
		Status:  true,
		Details: results,
	})
}
