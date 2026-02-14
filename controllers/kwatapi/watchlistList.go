// controllers/kwatapi/watchlistList.go
package kwatapi

import (
	"strconv"
	"strings"

	"klynx/internal/logger"
	"klynx/internal/services/kwatsvc"
	"klynx/models/kwatmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// watchlistList godoc
// @Summary List watchlists
// @Description Return only documents with **isDeleted=false**. Supports pagination, search, state, and warrant-level filters.
// @Tags Kwatch/Watchlist
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPages query int false "Items per page" default(10)
// @Param search query string false "Free-text search in personKey/firstname/lastname/nickname"
// @Param type query int false "Filter by person type (number)"
// @Param id query string false "Filter by _id (hex ObjectID)"
// @Param idcard query string false "Filter by ID card"
// @Param passport query string false "Filter by passport"
// @Param state query string false "Comma-separated states (e.g. active,queued,suspended)"
// @Param policeRegion query string false "Comma-separated regions found in warrants[].policeRegion (match within the same warrant item)"
// @Param policeProvincial query string false "Comma-separated provincials found in warrants[].policeProvincial"
// @Param policeStation query string false "Comma-separated stations found in warrants[].policeStation"
// @Param sortField query string false "Sort field (allowed: updatedAt,createdAt,firstname,lastname,nickname,idcard,personalType,type,rev)" default(updatedAt)
// @Param sortOrder query string false "Sort order" Enums(asc,desc) default(desc)
// @Success 200 {object} kwatmod.WatchlistPagination
// @Router /kwatch/watchlist [get]
// @Security BearerAuth
func WatchlistList(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/kwatapi")
	ctx, span := tracer.Start(ctx, "Kwatch.WatchlistList")
	defer span.End()

	log := logger.FromCtx(ctx, "kwatapi", "WatchlistList")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	// ค่าเริ่มต้นปลอดภัย: updatedAt desc
	sortField := strings.TrimSpace(c.Query("sortField", "updatedAt"))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder", "desc")))

	// รวบรวม filters (string map) ส่งเข้า service
	filters := map[string]string{
		"id":               strings.TrimSpace(c.Query("id")),
		"idcard":           strings.TrimSpace(c.Query("idcard")),
		"passport":         strings.TrimSpace(c.Query("passport")),
		"search":           strings.TrimSpace(c.Query("search")),
		"type":             strings.TrimSpace(c.Query("type")),
		"state":            strings.TrimSpace(c.Query("state")), // e.g. "active,queued"
		"policeRegion":     strings.TrimSpace(c.Query("policeRegion")),
		"policeProvincial": strings.TrimSpace(c.Query("policeProvincial")),
		"policeStation":    strings.TrimSpace(c.Query("policeStation")),
	}

	data, pag, err := kwatsvc.WatchlistList(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to list watchlists")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to list watchlists")
	}

	return kwatmod.SendPagination(c, data, pag)
}
