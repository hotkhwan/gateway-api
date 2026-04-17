// controllers/grpapi/get.go
package grpapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// ListGroups godoc
// @Summary      List groups
// @Description  Returns a paginated list of groups or subgroups under a parent
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPage     query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by"
// @Param        select      query     string  false  "subgroups (list children)"
// @Param        filter      query     string  false  "Parent group ID"
// @Param        search      query     string  false  "Search by name or ID"
// @Success      200   {object}  gmod.Pagination
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups [get]
// @Security     BearerAuth
func ListGroups(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.grpapi", "group.ListGroups", "grpapi", "ListGroups")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPage", "10"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "createdAt")

	selectType := c.Query("select")
	filterId := c.Query("filter")
	search := c.Query("search")

	filters := map[string]string{
		"select": selectType,
		"filter": filterId,
		"search": search,
	}

	log.Info().
		Int("page", page).
		Int("perPage", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListGroups] Listing groups")

	details, pagination, err := grpsvc.ListGroups(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListGroups] Failed to list groups")
		return httputil.FailInternal(c, "GROUP_LIST_FAILED", err.Error())
	}

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "groups fetched successfully",
		"status":     true,
		"details":    details,
		"pagination": pagination,
	})
}

// GetGroupTree godoc
// @Summary      Group tree with hierarchy and members
// @Description  Returns the full nested group hierarchy with each group's child and member users
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Success      200 {object} grpmod.GroupTreeResponse
// @Failure      500 {object} gmod.InternalErrorResponse
// @Router       /groups/tree [get]
// @Security     BearerAuth
func GetGroupTree(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.grpapi", "group.GetGroupTree", "grpapi", "GetGroupTree")
	defer end()

	// ดึงทั้งหมด
	groups, err := grpsvc.GetAllGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get groups")
		return httputil.FailInternal(c, "GET_GROUPS_FAILED", err.Error())
	}

	tree := grpsvc.BuildGroupTree(ctx, groups, nil)
	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "group tree fetched successfully",
		"status":  true,
		"details": tree,
	})
}
