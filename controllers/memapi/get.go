package memapi

import (
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/memsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// ListMembers godoc
// @Summary List members
// @Description Returns a paginated list of members
// @Tags Members
// @Accept  json
// @Produce  json
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page"
// @Param sortOrder query string false "asc or desc"
// @Param sortField query string false "Field to sort by"
// @Param filter query string false "Group ID"
// @Param search query string false "Search by name or ID"
// @Success 201 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members [get]
// @Security BearerAuth

func ListMembers(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.memapi", "memapi.ListMembers", "memapi", "ListMembers")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	if perPages <= 0 {
		perPages = 10
	}
	if perPages > 200 {
		perPages = 200
	}

	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "createdAt")

	filters := map[string]string{}
	if groupID := c.Query("groupID"); groupID != "" {
		filters["groupID"] = groupID
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📦 [MemberList] Fetching member list")

	members, pag, err := memsvc.ListMembers(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to list members")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_LIST_MEMBERS")
	}

	memPage := memmod.Pagination{
		Page:         pag.Page,
		PerPages:     pag.PerPages,
		TotalRecords: pag.TotalRecords,
		TotalPages:   pag.TotalPages,
		SortField:    pag.SortField,
		SortOrder:    pag.SortOrder,
	}

	log.Info().Int("count", len(members)).Msg("✅ Members listed")
	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "Members listed successfully",
		"status":     true,
		"details":    members,
		"pagination": memPage,
	})

}
