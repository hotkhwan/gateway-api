// controllers/mapapi/camera.go
package mapapi

import (
	"strconv"

	"github.com/hotkhwan/gateway-api/internal/services/mapsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeviceMap godoc
// @Summary      Device locations for map (clusters + devices)
// @Description  Return device locations for map by viewport (minLat, maxLat, minLong, maxLong, zoom)
// @Tags         Maps
// @Accept       json
// @Produce      json
// @Param        minLat   query     number  false  "Minimum latitude"
// @Param        maxLat   query     number  false  "Maximum latitude"
// @Param        minLong  query     number  false  "Minimum longitude"
// @Param        maxLong  query     number  false  "Maximum longitude"
// @Param        zoom     query     int     false  "Map zoom level"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /map/camera [get]
func DeviceMap(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/mapapi",
		"mapapi.DeviceMap",
		"mapapi",
		"DeviceMap",
	)
	defer span.End()

	// ---------- parse query ----------
	minLatStr := c.Query("minLat")
	maxLatStr := c.Query("maxLat")
	minLngStr := c.Query("minLng")
	maxLngStr := c.Query("maxLng")
	zoomStr := c.Query("zoom")

	var (
		minLat, maxLat float64
		minLng, maxLng float64
		err            error
	)

	hasBounds := false

	if minLatStr != "" || maxLatStr != "" || minLngStr != "" || maxLngStr != "" {
		// ต้องครบทั้ง 4 ตัวเพื่อให้ filter ตาม viewport
		if minLatStr == "" || maxLatStr == "" || minLngStr == "" || maxLngStr == "" {
			log.Warn().Msg("❌ invalid bounds: some of minLat/maxLat/minLng/maxLng missing")
			return httputil.FailBadRequest(c, "INVALID_BOUNDS", "minLat,maxLat,minLng,maxLng must all be provided or all be empty")
		}

		minLat, err = strconv.ParseFloat(minLatStr, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_MIN_LAT", "minLat must be a number")
		}
		maxLat, err = strconv.ParseFloat(maxLatStr, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_MAX_LAT", "maxLat must be a number")
		}
		minLng, err = strconv.ParseFloat(minLngStr, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_MIN_LNG", "minLng must be a number")
		}
		maxLng, err = strconv.ParseFloat(maxLngStr, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_MAX_LNG", "maxLng must be a number")
		}
		hasBounds = true
	}

	zoom := 15 // default ถ้าไม่ส่ง -> ถือว่า zoom ค่อนข้างใกล้
	if zoomStr != "" {
		if z, err := strconv.Atoi(zoomStr); err == nil && z > 0 {
			zoom = z
		}
	}

	log.Info().
		Bool("hasBounds", hasBounds).
		Float64("minLat", minLat).
		Float64("maxLat", maxLat).
		Float64("minLng", minLng).
		Float64("maxLng", maxLng).
		Int("zoom", zoom).
		Msg("📡 [DeviceMap] incoming map request")

	details, err := mapsvc.GetDevicesMap(ctx, hasBounds, minLat, maxLat, minLng, maxLng, zoom)
	if err != nil {
		log.Error().Err(err).Msg("❌ [DeviceMap] failed to get devices map")
		return httputil.FailInternal(c, "DEVICE_MAP_FAILED", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"details": details,
	})
}
