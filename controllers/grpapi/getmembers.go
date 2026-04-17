// controllers/grpapi/getmembers.go
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

// ListGroupMembers godoc
// @Summary      List members in group with pagination
// @Description  Returns a paginated list of members under a group
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPage     query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by"
// @Param        filter      query     string  false  "Group ID"
// @Param        search      query     string  false  "Search by username, email, name or ID"
// @Success      200   {object}  gmod.MembersResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups/members [get]
// @Security     BearerAuth
func ListGroupMembers(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.grpapi", "group.ListGroupMembers", "grpapi", "ListGroupMembers")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPage", "10"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "createdAt")

	filterId := c.Query("filter")
	search := c.Query("search")

	filters := map[string]string{
		"filter": filterId,
		"search": search,
	}

	log.Info().
		Int("page", page).
		Int("perPage", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListGroupMembers] Listing members")

	details, pagination, err := grpsvc.ListGroupMembers(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListGroupMembers] Failed to list members")
		return httputil.FailInternal(c, "MEMBER_LIST_FAILED", err.Error())
	}

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "members fetched successfully",
		"status":     true,
		"details":    details,
		"pagination": pagination,
	})
}
