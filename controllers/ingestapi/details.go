package ingestapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

type EventDetailsController struct {
	service *ingestsvc.ApprovalService
}

func NewEventDetailsController(service *ingestsvc.ApprovalService) *EventDetailsController {
	if service == nil {
		panic("ApprovalService required")
	}
	return &EventDetailsController{service: service}
}

func (ctrl *EventDetailsController) mustLocals(c *fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ListApprovedEvents lists approved events
// GET /api/v1/ingest/details
func (ctrl *EventDetailsController) ListApprovedEvents(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)

	input := ingestmod.ListEventsInput{
		TenantId:  tenantId,
		OrgId:     orgId,
		EventType: c.Query("eventType", ""),
		Page:      c.QueryInt("page", 1),
		PerPage:   c.QueryInt("perPage", 10),
		SortField: c.Query("sortField", "approvedAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	}

	result, err := ctrl.service.ListApproved(c.UserContext(), &input)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"code":    "INTERNAL_ERROR",
			"message": "internal server error",
			"status":  false,
		})
	}

	pagination := gmod.Pagination{
		Page:         input.Page,
		PerPages:     input.PerPage,
		TotalRecords: int(result.Total),
		TotalPages:   result.TotalPages,
		SortField:    input.SortField,
		SortOrder:    input.SortOrder,
	}

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "events fetched successfully",
		"status":     true,
		"details":    result.Items,
		"pagination": pagination,
	})
}

// GetApprovedEvent gets a single approved event
// GET /api/v1/ingest/details/:eventId
func (ctrl *EventDetailsController) GetApprovedEvent(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	event, err := ctrl.service.GetApprovedEvent(c.UserContext(), tenantId, orgId, eventId)
	if err != nil {
		code := gmod.CodeInternalError
		if err == ingestsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		}
		return c.Status(mapCodeToStatus(code)).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "event fetched successfully",
		"status":  true,
		"detail":  event,
	})
}

// Helper function to map error codes to HTTP status
func mapCodeToStatus(code string) int {
	switch code {
	case gmod.CodeBadRequest:
		return fiber.StatusBadRequest
	case gmod.CodeUnauthorized:
		return fiber.StatusUnauthorized
	case gmod.CodeForbidden:
		return fiber.StatusForbidden
	case gmod.CodeNotFound:
		return fiber.StatusNotFound
	case gmod.CodeConflict:
		return fiber.StatusConflict
	case gmod.CodeTooMany:
		return fiber.StatusTooManyRequests
	case gmod.CodeUnavailable:
		return fiber.StatusServiceUnavailable
	case gmod.CodeTimeout:
		return fiber.StatusRequestTimeout
	default:
		return fiber.StatusInternalServerError
	}
}
