// controllers/ingestapi/details.go
package ingestapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
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

func (ctrl *EventDetailsController) mustLocals(c fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ListApprovedEvents godoc
// @Summary      List approved events
// @Description  Returns paginated list of approved ingest events, optionally filtered by eventType.
// @Tags         ingest-details
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        eventType     query     string  false  "Filter by event type"
// @Param        page          query     int     false  "Page number"    default(1)
// @Param        perPage       query     int     false  "Items per page" default(10)
// @Param        sortField     query     string  false  "Sort field"     default(approvedAt)
// @Param        sortOrder     query     string  false  "Sort order"     default(desc)
// @Success      200  {object}  gmod.PaginatedResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/details [get]
func (ctrl *EventDetailsController) ListApprovedEvents(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventDetailsController.ListApprovedEvents", "ingestapi", "ListApprovedEvents")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)

	input := ingestmod.ListEventsInput{
		TenantId:  tenantId,
		OrgId:     orgId,
		EventType: c.Query("eventType", ""),
		Page:      fiber.Query[int](c, "page", 1),
		PerPage:   fiber.Query[int](c, "perPage", 10),
		SortField: c.Query("sortField", "approvedAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	}

	// Debug — read-only list query
	log.Debug().
		Str("orgId", orgId).
		Str("eventType", input.EventType).
		Int("page", input.Page).
		Msg("📥 [ListApprovedEvents] list approved events")

	result, err := ctrl.service.ListApproved(ctx, &input)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("❌ [ListApprovedEvents] failed to list events")
		return httputil.FailInternal(c, "failed to list approved events")
	}

	pagination := gmod.Pagination{
		Page:         input.Page,
		PerPages:     input.PerPage,
		TotalRecords: int(result.Total),
		TotalPages:   result.TotalPages,
		SortField:    input.SortField,
		SortOrder:    input.SortOrder,
	}

	// Debug — read-only result
	log.Debug().Str("orgId", orgId).Int64("total", result.Total).Msg("✅ [ListApprovedEvents] events fetched")

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "events fetched successfully",
		"status":     true,
		"details":    result.Items,
		"pagination": pagination,
	})
}

// GetApprovedEvent godoc
// @Summary      Get approved event
// @Description  Returns a single approved ingest event by ID.
// @Tags         ingest-details
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        eventId       path      string  true  "Event ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/details/{eventId} [get]
func (ctrl *EventDetailsController) GetApprovedEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventDetailsController.GetApprovedEvent", "ingestapi", "GetApprovedEvent")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	// Debug — read-only single fetch
	log.Debug().Str("orgId", orgId).Str("eventId", eventId).Msg("📥 [GetApprovedEvent] get event")

	event, err := ctrl.service.GetApprovedEvent(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestsvc.ErrEventNotFound {
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [GetApprovedEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		}
		log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [GetApprovedEvent] internal error")
		return httputil.FailInternal(c, "failed to get event")
	}

	// Debug — read-only result
	log.Debug().Str("orgId", orgId).Str("eventId", eventId).Msg("✅ [GetApprovedEvent] event fetched")

	return httputil.Ok(c, event, "event fetched successfully")
}
