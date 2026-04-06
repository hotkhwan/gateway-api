// controllers/mapapi/kml.go
package mapapi

import (
	"strconv"

	"github.com/hotkhwan/gateway-api/internal/services/mapsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// GetKML godoc
// @Summary Get KML files from S3 with pagination and metadata
// @Tags Maps
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page"
// @Param sortOrder query string false "asc or desc"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /maps/kml [get]
// @Security BearerAuth
func GetMAPPosition(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.mapapi", "GetMAPPosition.GetMAPPosition", "mapapi", "GetMAPPosition")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortOrder := c.Query("sortOrder", "desc")

	filters := map[string]string{}
	if id := c.Query("id"); id != "" {
		filters["id"] = id
	}
	if code := c.Query("code"); code != "" {
		filters["code"] = code
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	log.Info().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListMAP] Listing map position")

	details, pagination, err := mapsvc.ListMAPPosition(ctx, page, perPages, filters, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListMAP] Failed to map position list")
		return httputil.FailInternal(c, err.Error())
	}

	log.Info().
		Int("resultCount", len(details)).
		Interface("pagination", pagination).
		Msg("✅ [ListMAP] Device list fetched successfully")

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "Map position list fetched successfully",
		"status":     true,
		"details":    details,
		"pagination": pagination,
	})
}
