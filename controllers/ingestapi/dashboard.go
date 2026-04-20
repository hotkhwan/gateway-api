// controllers/ingestapi/dashboard.go
package ingestapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingeststatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type IngestDashboardController struct {
	service *ingeststatsvc.DashboardStatsService
}

func NewIngestDashboardController(service *ingeststatsvc.DashboardStatsService) *IngestDashboardController {
	return &IngestDashboardController{service: service}
}

// GetIngestDashboard godoc
// @Summary      Get ingest dashboard stats
// @Description  Returns aggregated ingest statistics filtered by date range, status, and event type.
// @Tags         ingest-dashboard
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        dateTime      query     string  false  "Date range: from,to (RFC3339 UTC, e.g. 2026-03-01T00:00:00Z,2026-03-06T23:59:59Z)"
// @Param        status        query     string  false  "Filter by status (all|pending|approved|rejected)"  default(all)
// @Param        eventType     query     string  false  "Filter by event type"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dashboard [get]
func (ctrl *IngestDashboardController) GetIngestDashboard(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "IngestDashboardController.GetIngestDashboard", "ingestapi", "GetIngestDashboard")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)

	// Parse dateTime=from,to (RFC3339 UTC)
	startDate, endDate := "", ""
	if dt := c.Query("dateTime", ""); dt != "" {
		parts := strings.SplitN(dt, ",", 2)
		if len(parts) == 2 {
			startDate = strings.TrimSpace(parts[0])
			endDate = strings.TrimSpace(parts[1])
		}
	}

	input := &ingestmod.GetIngestDashboardInput{
		TenantId:  tenantId,
		OrgId:     orgId,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    c.Query("status", "all"),
		EventType: c.Query("eventType", ""),
	}

	// Debug — read-only query
	log.Debug().
		Str("orgId", orgId).
		Str("startDate", input.StartDate).
		Str("endDate", input.EndDate).
		Str("status", input.Status).
		Str("eventType", input.EventType).
		Msg("📥 [GetIngestDashboard] fetching dashboard stats")

	stats, err := ctrl.service.GetIngestDashboardStats(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, ingeststatsvc.ErrInvalidDateRange):
			log.Warn().Str("orgId", orgId).Err(err).Msg("❌ [GetIngestDashboard] invalid date range")
			return httputil.FailBadRequest(c, "invalid date range: "+err.Error())
		case errors.Is(err, ingeststatsvc.ErrDateRangeTooLong):
			log.Warn().Str("orgId", orgId).Err(err).Msg("❌ [GetIngestDashboard] date range too long")
			return httputil.FailBadRequest(c, "date range too long: "+err.Error())
		default:
			log.Error().Err(err).Str("orgId", orgId).Msg("❌ [GetIngestDashboard] internal error")
			return httputil.FailInternal(c, "failed to get dashboard stats")
		}
	}

	// Debug — read-only result
	log.Debug().Str("orgId", orgId).Msg("✅ [GetIngestDashboard] stats fetched")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "dashboard stats retrieved successfully",
		"status":  true,
		"details": stats,
	})
}
