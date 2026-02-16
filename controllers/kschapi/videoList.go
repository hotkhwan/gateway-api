// controllers/kschapi/videoList.go
package kschapi

import (
	"strconv"

	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// videoList godoc
// @Summary List videos
// @Description List videos with pagination (default state=created) and signed S3 URL
// @Tags Ksearch/Video
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPages query int false "Items per page" default(10)
// @Param search query string false "Search by name"
// @Param state query string false "Filter by state" default("created")
// @Param status query string false "Filter by status true/false"
// @Param sortField query string false "Sort field" default("createdAt")
// @Param sortOrder query string false "Sort order" Enums(asc,desc) default(desc)
// @Success 200 {object} gmod.PaginationResponse
// @Router /ksearch/video [get]
// @Security BearerAuth
func VideoList(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/kschapi", "search.VideoList", "kschapi", "VideoList")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	filters := map[string]string{
		"search": c.Query("search"),
		"state":  c.Query("state"),
		"status": c.Query("status"),
	}
	id := c.Params("id")
	data, pagination, err := kschsvc.VideoList(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to get watchlist")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to list videos")
	}

	// ✅ ใช้รูปแบบ PaginationDbResponse<T> เหมือนเดิม
	return gmod.SendPagination(c, data, pagination)
	// return gmod.SendPagination[[]kschmod.VideoResponse](c, data, pagination)
}
