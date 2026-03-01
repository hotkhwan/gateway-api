package eventapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/eventsvc"
	"github.com/hotkhwan/gateway-api/models/eventmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
)

type EventManagementController struct {
	service *eventsvc.ApprovalService
}

func NewEventManagementController(service *eventsvc.ApprovalService) *EventManagementController {
	if service == nil {
		panic("ApprovalService required")
	}
	return &EventManagementController{service: service}
}

func (ctrl *EventManagementController) mustLocals(c *fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ListPendingEvents lists pending events
// GET /api/v1/ingest/management
func (ctrl *EventManagementController) ListPendingEvents(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)

	input := eventmod.ListEventsInput{
		TenantId:   tenantId,
		OrgId:      orgId,
		StatusName:  c.Query("status", "pending"), // pending|approved|rejected|all
		EventType:   c.Query("eventType", ""),
		Page:       c.QueryInt("page", 1),
		PerPage:    c.QueryInt("perPage", 10),
		SortField:  c.Query("sortField", "createdAt"),
		SortOrder:  c.Query("sortOrder", "desc"),
	}

	result, err := ctrl.service.ListPending(c.UserContext(), &input)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
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

// GetPendingEvent gets a single pending event
// GET /api/v1/ingest/management/:eventId
func (ctrl *EventManagementController) GetPendingEvent(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	event, err := ctrl.service.GetPendingEvent(c.UserContext(), tenantId, orgId, eventId)
	if err != nil {
		code := gmod.CodeInternalError
		if err == eventsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		}
		return c.Status(mapCodeToStatusMgmt(code)).JSON(gmod.ApiErrorResponse{
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

// UpdatePendingEvent updates pending event metadata
// PATCH /api/v1/ingest/management/:eventId
func (ctrl *EventManagementController) UpdatePendingEvent(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	var body eventmod.EventUpdateInput
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	if err := ctrl.service.UpdatePendingEvent(
		c.UserContext(),
		tenantId, orgId, eventId, callerUserId,
		&body,
	); err != nil {
		code := gmod.CodeInternalError
		if err == eventsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		} else if err == eventsvc.ErrEventAlreadyApproved {
			code = gmod.CodeConflict
		}
		return c.Status(mapCodeToStatusMgmt(code)).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return httputil.MessageOK(c, "event updated successfully")
}

// ApproveEvent approves a pending event
// POST /api/v1/ingest/management/:eventId/approve
func (ctrl *EventManagementController) ApproveEvent(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	var body eventmod.EventUpdateInput
	// Optional metadata update during approval
	c.BodyParser(&body)

	eventDetail, err := ctrl.service.ApproveEvent(
		c.UserContext(),
		tenantId, orgId, eventId, callerUserId,
		&body,
	)
	if err != nil {
		code := gmod.CodeInternalError
		if err == eventsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		} else if err == eventsvc.ErrEventAlreadyApproved {
			code = gmod.CodeConflict
		} else if err == eventsvc.ErrEventAlreadyRejected {
			code = gmod.CodeConflict
		}
		return c.Status(mapCodeToStatusMgmt(code)).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return httputil.Ok(c, eventDetail, "event approved successfully")
}

// RejectEvent rejects a pending event
// POST /api/v1/ingest/management/:eventId/reject
func (ctrl *EventManagementController) RejectEvent(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	if err := ctrl.service.RejectEvent(
		c.UserContext(),
		tenantId, orgId, eventId, callerUserId,
	); err != nil {
		code := gmod.CodeInternalError
		if err == eventsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		} else if err == eventsvc.ErrEventAlreadyApproved {
			code = gmod.CodeConflict
		}
		return c.Status(mapCodeToStatusMgmt(code)).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return httputil.MessageOK(c, "event rejected successfully")
}

// DeletePendingEvent deletes a pending event
// DELETE /api/v1/ingest/management/:eventId
func (ctrl *EventManagementController) DeletePendingEvent(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	if err := ctrl.service.DeletePendingEvent(
		c.UserContext(),
		tenantId, orgId, eventId, callerUserId,
	); err != nil {
		code := gmod.CodeInternalError
		if err == eventsvc.ErrEventNotFound {
			code = gmod.CodeNotFound
		} else if err == eventsvc.ErrEventAlreadyApproved {
			code = gmod.CodeConflict
		}
		return c.Status(mapCodeToStatusMgmt(code)).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return httputil.MessageOK(c, "event deleted successfully")
}

// Helper function to map error codes to HTTP status
func mapCodeToStatusMgmt(code string) int {
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
