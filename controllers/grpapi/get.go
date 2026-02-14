// controllers/grpapi/get.go
package grpapi

import (
	"klynx/internal/services/grpsvc"
	"klynx/models/gmod"
	"klynx/models/grpmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListGroups godoc
// @Summary      List groups
// @Description  Returns a paginated list of groups or subgroups under a parent
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPages    query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by"
// @Param        select      query     string  false  "subgroups (list children)"
// @Param        filter      query     string  false  "Parent group ID"
// @Param        search      query     string  false  "Search by name or ID"
// @Success      200   {object}  gmod.Pagination
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups [get]
// @Security     BearerAuth
func ListGroups(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/grpapi", "group.ListGroups", "grpapi", "ListGroups")
	defer span.End()
	// ctx := c.UserContext()
	// tracer := otel.Tracer("klynx/grpapi")
	// ctx, span := tracer.Start(ctx, "groups.ListGroups")
	// defer span.End()

	// if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
	// 	ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	// }

	// log := logger.FromCtx(ctx, "kwatapi", "WatchlistCreate")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
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
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListGroups] Listing groups")

	details, pagination, err := grpsvc.ListGroups(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListGroups] Failed to list groups")
		return httputil.FailInternal(c, "GROUP_LIST_FAILED", err.Error())
	}

	return gmod.SendPagination(c, details, pagination)
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
func GetGroupTree(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/grpapi", "group.GetGroupTree", "grpapi", "GetGroupTree")
	defer span.End()

	// ดึงทั้งหมด
	groups, err := grpsvc.GetAllGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get groups")
		return httputil.FailInternal(c, "GET_GROUPS_FAILED", err.Error())
	}

	tree := grpsvc.BuildGroupTree(ctx, groups, nil)
	return c.JSON(grpmod.GroupTreeResponse{
		Details: tree, // << ใช้ "details" ตาม struct
		Status:  true,
	})
}
