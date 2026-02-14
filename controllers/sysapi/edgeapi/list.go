// controllers/sysapi/edgeapi/list.go
package edgeapi

import (
	"math"
	"strconv"
	"strings"

	"klynx/internal/logger"
	"klynx/internal/services/systemsvc/edgesvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

func ListEdges(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"klynx/edgeapi",
		"system.edge.List",
		"edgeapi",
		"ListEdges",
	)
	defer span.End()
	_ = logger.FromCtx(ctx, "edgeapi", "ListEdges")

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
		// normalize: code=INTERNAL_ERROR, reason=LIST_FAILED
		return httputil.FailInternalReason(c, "internal server error", "LIST_FAILED")
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return gmod.SendPaginationOK(
		c,
		items,
		gmod.Pagination{
			Page:         page,
			PerPages:     perPage,
			TotalRecords: int(total),
			TotalPages:   totalPages,
			SortField:    "updatedAt",
			SortOrder:    "desc",
		},
	)
}
