// controllers/sysapi/edgeapi/list.go
package edgeapi

import (
	"math"
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/edgesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

func ListEdges(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.edgeapi", "EdgeApi.ListEdges", "sysapi", "ListEdges")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("perPage", "20"))

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}

	items, total, err := edgesvc.ListEdges(ctx, edgesvc.ListQuery{
		Q:       strings.TrimSpace(c.Query("q", "")),
		Type:    strings.TrimSpace(c.Query("type", "")),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		log.Error().Err(err).Msg("list edges failed")
		return httputil.FailInternalReason(c, "internal server error", "LIST_FAILED")
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "ok",
		"status":  true,
		"details": items,
		"pagination": gmod.Pagination{
			Page:         page,
			PerPage:     perPage,
			TotalRecords: int(total),
			TotalPages:   totalPages,
			SortField:    "updatedAt",
			SortOrder:    "desc",
		},
	})
}
