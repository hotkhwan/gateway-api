// controllers/authzapi/orgListMembers.go
package authzapi

import (
	"math"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ListMembers godoc
// @Summary List organization members
// @Description Return list of members in active organization (admin only)
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param X-Active-Org header string true "Active Organization ID"
// @Success 200 {object} gmod.OrgListResponse
// @Failure 403 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/users/members [get]
func (ctrl *OrganizationController) ListMembers(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.authzapi", "OrganizationController.ListMembers", "authzapi", "ListMembers")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))

	sizeStr := c.Query("perPages", "")
	if sizeStr == "" {
		sizeStr = c.Query("size", "10")
	}
	size, _ := strconv.Atoi(sizeStr)
	search := c.Query("search", "")
	sortField := c.Query("sortField", "firstName")
	sortOrder := c.Query("sortOrder", "asc")

	members, totalRecords, err := ctrl.service.ListMembers(
		ctx,
		tenantId,
		orgId,
		callerUserId,
		page,
		size,
		search,
		sortField,
		sortOrder,
	)
	if err != nil {
		return httputil.FailForbidden(c, err.Error())
	}

	totalPages := 0
	if size > 0 {
		totalPages = int(math.Ceil(float64(totalRecords) / float64(size)))
	}

	response := gmod.OrgListResponse{
		Code:    gmod.CodeSuccess,
		Message: "List member in org",
		Status:  true,
		Details: members,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     size,
			TotalRecords: totalRecords,
			TotalPages:   totalPages,
			SortField:    sortField,
			SortOrder:    sortOrder,
		},
	}

	return c.JSON(response)
}
