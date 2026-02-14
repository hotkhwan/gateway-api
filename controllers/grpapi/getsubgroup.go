// controllers/grpapi/getsubgroup.go
package grpapi

import (
	"klynx/internal/services/grpsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListSubGroups godoc
// @Summary      List subgroups with pagination
// @Description  Return a paginated list of subgroups under the given parentId
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPages    query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by (default: createdAt)"
// @Param        filter      query     string  true   "Parent group ID (alias: parentId)"
// @Param        parentId    query     string  false  "Parent group ID (alias of filter)"
// @Param        search      query     string  false  "Search by name or _id"
// @Success      200         {object}  gmod.GroupResponse
// @Failure      400         {object}  gmod.ErrorMessageResponse "PARENT_ID_REQUIRED"
// @Failure      500         {object}  gmod.InternalErrorResponse
// @Router       /groups/subgroups [get]
// @Security     BearerAuth
func ListSubGroups(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/grpapi", "group.ListSubGroups", "grpapi", "ListSubGroups")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "createdAt")

	// รับทั้ง filter และ parentId (alias กัน)
	parentId := c.Query("filter")
	if parentId == "" {
		parentId = c.Query("parentId")
	}
	search := c.Query("search")

	// ต้องมี parentId เสมอสำหรับ list subgroups
	if strings.TrimSpace(parentId) == "" {
		return httputil.FailBadRequest(c, "PARENT_ID_REQUIRED", "query 'filter' or 'parentId' is required")
	}

	filters := map[string]string{
		"select": "subgroups",
		"filter": parentId, // parentId ที่ใช้คัดกรอง
		"search": search,
	}

	log.Info().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Str("parentId", parentId).
		Str("search", search).
		Msg("📡 [ListSubGroups] Listing subgroups by parentId")

	details, pagination, err := grpsvc.ListSubGroups(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListSubGroups] Failed to list subgroups")
		return httputil.FailInternal(c, "GROUP_LIST_FAILED", err.Error())
	}

	return gmod.SendPagination(c, details, pagination)
}
