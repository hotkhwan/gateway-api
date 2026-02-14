// controllers/grpapi/getdevices.go
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

// ListGroupDevices godoc
// @Summary      List devices under a group with pagination
// @Description  Returns a paginated list of devices under a parent group
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        page        query     int     false  "Page number"
// @Param        perPages    query     int     false  "Items per page"
// @Param        sortOrder   query     string  false  "asc or desc"
// @Param        sortField   query     string  false  "Field to sort by"
// @Param        filter      query     string  false  "Parent group ID"
// @Param        search      query     string  false  "Search by name or ID"
// @Success      200   {object}  gmod.DeviceResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups/devices [get]
// @Security     BearerAuth
func ListGroupDevices(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/grpapi", "group.ListGroupDevices", "grpapi", "ListGroupDevices")
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
		Msg("📡 [ListGroupDevices] Listing devices")

	details, pagination, err := grpsvc.ListGroupDevices(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListGroupDevices] Failed to list devices")
		return httputil.FailInternal(c, "DEVICE_LIST_FAILED", err.Error())
	}

	return gmod.SendPagination(c, details, pagination)
}
