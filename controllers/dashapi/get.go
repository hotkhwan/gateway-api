// controllers/dashapi/get.go
package dashapi

import (
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/dashsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DashboardSummary godoc
// @Summary      Kcontrol dashboard summary (events + peopleCountingArea)
// @Description  Return dashboard summary from ata_events (event stats, peopleCountingArea, notification, backlist)
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        dateTime  query     string  false  "Date range in format YYYY-MM-DD,YYYY-MM-DD (start,end). Default: today 00:00 to now (Asia/Bangkok)"
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/dashboard [get]
func DashboardSummary(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.dashapi", "Dashboard.DashboardSummary", "dashapi", "DashboardSummary")
	defer end()

	// ----------- parse dateTime query param -----------
	loc, _ := time.LoadLocation("Asia/Bangkok")
	if loc == nil {
		loc = time.UTC
	}

	var startTime, endTime time.Time

	dateParam := c.Query("dateTime", "")
	now := time.Now().In(loc)

	if dateParam == "" {
		// default: today 00:00 ถึงตอนนี้
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		endTime = now
	} else {
		parts := strings.Split(dateParam, ",")
		startStr := strings.TrimSpace(parts[0])

		startDate, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			log.Error().Err(err).Str("dateTime", dateParam).Msg("❌ invalid dateTime format, fallback to today")
			startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			endTime = now
		} else {
			var endDate time.Time
			if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
				endStr := strings.TrimSpace(parts[1])
				ed, err2 := time.ParseInLocation("2006-01-02", endStr, loc)
				if err2 != nil {
					log.Error().Err(err2).Str("dateTime", dateParam).Msg("❌ invalid end date, use start date only")
					endDate = startDate
				} else {
					endDate = ed
				}
			} else {
				// ถ้าไม่ส่ง end มา ใช้วันเดียวกับ start
				endDate = startDate
			}

			// start = 00:00 ของวันแรก
			startTime = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, loc)
			// end = วันถัดไป 00:00 (ใช้เป็น exclusive bound)
			endTime = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		}
	}

	log.Info().
		Time("start", startTime).
		Time("end", endTime).
		Msg("📡 [DashboardSummary] building dashboard details from ata_events with dateTime range")

	details, err := dashsvc.BuildAtaDashboard(ctx, startTime, endTime)
	if err != nil {
		log.Error().Err(err).Msg("❌ [DashboardSummary] failed to build dashboard summary")
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, details, "dashboard summary")
}
