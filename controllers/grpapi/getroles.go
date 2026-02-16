// controllers/grpapi/getroles.go
package grpapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListGroupRoles godoc
// @Summary      List roles in group with pagination
// @Description  Returns a paginated list of roles under a group
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPages    query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by"
// @Param        filter      query     string  false  "Group ID"
// @Param        search      query     string  false  "Search by role name or ID"
// @Success      200   {object}  gmod.RolesResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups/roles [get]
// @Security     BearerAuth
func ListGroupRoles(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/grpapi", "group.ListGroupRoles", "grpapi", "ListGroupRoles")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
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
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListGroupRoles] Listing roles")

	details, pagination, err := grpsvc.ListGroupRoles(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListGroupRoles] Failed to list roles")
		return httputil.FailInternal(c, "ROLE_LIST_FAILED", err.Error())
	}

	return gmod.SendPagination(c, details, pagination)
}
