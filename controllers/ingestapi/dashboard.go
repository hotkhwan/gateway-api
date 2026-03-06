package ingestapi

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/ingeststatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

type IngestDashboardController struct {
	service *ingeststatsvc.DashboardStatsService
}

func NewIngestDashboardController(service *ingeststatsvc.DashboardStatsService) *IngestDashboardController {
	return &IngestDashboardController{service: service}
}

// GetIngestDashboard ดึงข้อมูล ingest dashboard stats
// GET /api/v1/ingest/dashboard?startDate=2026-01-20T00:00:00Z&endDate=2026-01-20T16:59:59Z&status=pending&eventType=LPR_Brand
func (ctrl *IngestDashboardController) GetIngestDashboard(c *fiber.Ctx) error {
	// Get tenant/org from JWT locals
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	input := &ingestmod.GetIngestDashboardInput{
		TenantId:  tenantId,
		OrgId:     orgId,
		StartDate: c.Query("startDate", ""),
		EndDate:   c.Query("endDate", ""),
		Status:    c.Query("status", "all"), // all, pending, approved, rejected
		EventType: c.Query("eventType", ""),
	}

	stats, err := ctrl.service.GetIngestDashboardStats(c.UserContext(), input)
	if err != nil {
		// Map error to appropriate status code
		var httpStatus int
		var code string

		switch {
		case errors.Is(err, ingeststatsvc.ErrInvalidDateRange):
			httpStatus = fiber.StatusBadRequest
			code = gmod.CodeBadRequest
		case errors.Is(err, ingeststatsvc.ErrDateRangeTooLong):
			httpStatus = fiber.StatusBadRequest
			code = gmod.CodeBadRequest
		default:
			httpStatus = fiber.StatusInternalServerError
			code = gmod.CodeInternalError
		}

		return c.Status(httpStatus).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: "failed to get dashboard stats: " + err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "dashboard stats retrieved successfully",
		"status":  true,
		"details": stats,
	})
}
