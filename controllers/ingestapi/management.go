// controllers/ingestapi/management.go
package ingestapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type EventManagementController struct {
	service *ingestsvc.ApprovalService
}

func NewEventManagementController(service *ingestsvc.ApprovalService) *EventManagementController {
	if service == nil {
		panic("ApprovalService required")
	}
	return &EventManagementController{service: service}
}

func (ctrl *EventManagementController) mustLocals(c fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ListPendingEvents godoc
// @Summary      List pending events
// @Description  Returns paginated list of ingest events filtered by status, eventType, and sort options.
// @Tags         ingest-management
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        status        query     string  false  "Event status (all|pending|approved|rejected)"  default(all)
// @Param        eventType     query     string  false  "Filter by event type"
// @Param        page          query     int     false  "Page number"    default(1)
// @Param        perPage       query     int     false  "Items per page" default(10)
// @Param        sortField     query     string  false  "Sort field"     default(createdAt)
// @Param        sortOrder     query     string  false  "Sort order"     default(desc)
// @Success      200  {object}  gmod.PaginatedResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management [get]
func (ctrl *EventManagementController) ListPendingEvents(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/ingestapi", "EventManagementController.ListPendingEvents", "ingestapi", "ListPendingEvents")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)

	input := ingestmod.ListEventsInput{
		TenantId:   tenantId,
		OrgId:      orgId,
		StatusName: c.Query("status", "all"),
		EventType:  c.Query("eventType", ""),
		Page:       fiber.Query[int](c, "page", 1),
		PerPage:    fiber.Query[int](c, "perPage", 10),
		SortField:  c.Query("sortField", "createdAt"),
		SortOrder:  c.Query("sortOrder", "desc"),
	}

	// Debug — read-only list query
	log.Debug().
		Str("orgId", orgId).
		Str("status", input.StatusName).
		Str("eventType", input.EventType).
		Int("page", input.Page).
		Msg("📥 [ListPendingEvents] list events")

	result, err := ctrl.service.ListPending(ctx, &input)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Msg("❌ [ListPendingEvents] failed to list events")
		return httputil.FailInternal(c, "failed to list events")
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
	log.Debug().Str("orgId", orgId).Int64("total", result.Total).Msg("✅ [ListPendingEvents] events fetched")

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "events fetched successfully",
		"status":     true,
		"details":    result.Items,
		"pagination": pagination,
	})
}

// GetPendingEvent godoc
// @Summary      Get pending event
// @Description  Returns a single ingest event by ID.
// @Tags         ingest-management
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        eventId       path      string  true  "Event ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/{eventId} [get]
func (ctrl *EventManagementController) GetPendingEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventManagementController.GetPendingEvent", "ingestapi", "GetPendingEvent")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	// Debug — read-only single fetch
	log.Debug().Str("orgId", orgId).Str("eventId", eventId).Msg("📥 [GetPendingEvent] get event")

	event, err := ctrl.service.GetPendingEvent(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestsvc.ErrEventNotFound {
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [GetPendingEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		}
		log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [GetPendingEvent] internal error")
		return httputil.FailInternal(c, "failed to get event")
	}

	// Debug — read-only result
	log.Debug().Str("orgId", orgId).Str("eventId", eventId).Msg("✅ [GetPendingEvent] event fetched")

	return httputil.Ok(c, event, "event fetched successfully")
}

// UpdatePendingEvent godoc
// @Summary      Update pending event
// @Description  Patches metadata fields on a pending event.
// @Tags         ingest-management
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string                      true  "Active Org ID"
// @Param        eventId       path      string                      true  "Event ID"
// @Param        body          body      ingestmod.EventUpdateInput  true  "Fields to update"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      409  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/{eventId} [patch]
func (ctrl *EventManagementController) UpdatePendingEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventManagementController.UpdatePendingEvent", "ingestapi", "UpdatePendingEvent")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	var body ingestmod.EventUpdateInput
	if err := c.Bind().Body(&body); err != nil {
		log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [UpdatePendingEvent] invalid body")
		return httputil.FailBadRequest(c, "invalid body")
	}

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✏️ [UpdatePendingEvent] updating event")

	if err := ctrl.service.UpdatePendingEvent(ctx, tenantId, orgId, eventId, callerUserId, &body); err != nil {
		switch err {
		case ingestsvc.ErrEventNotFound:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [UpdatePendingEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		case ingestsvc.ErrEventAlreadyApproved:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [UpdatePendingEvent] event already approved")
			return httputil.FailConflict(c, "event already approved")
		default:
			log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [UpdatePendingEvent] internal error")
			return httputil.FailInternal(c, "failed to update event")
		}
	}

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✅ [UpdatePendingEvent] event updated")

	return httputil.MessageOK(c, "event updated successfully")
}

// ApproveEvent godoc
// @Summary      Approve pending event
// @Description  Approves a pending event and triggers downstream processing. Optionally updates metadata in the same request.
// @Tags         ingest-management
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string                      true   "Active Org ID"
// @Param        eventId       path      string                      true   "Event ID"
// @Param        body          body      ingestmod.EventUpdateInput  false  "Optional metadata update"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      409  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/{eventId}/approve [post]
func (ctrl *EventManagementController) ApproveEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventManagementController.ApproveEvent", "ingestapi", "ApproveEvent")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	var body ingestmod.EventUpdateInput
	// Optional metadata update during approval
	c.Bind().Body(&body) //nolint:errcheck

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✏️ [ApproveEvent] approving event")

	eventDetail, err := ctrl.service.ApproveEvent(ctx, tenantId, orgId, eventId, callerUserId, &body)
	if err != nil {
		switch err {
		case ingestsvc.ErrEventNotFound:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [ApproveEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		case ingestsvc.ErrEventAlreadyApproved, ingestsvc.ErrEventAlreadyRejected:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Err(err).Msg("❌ [ApproveEvent] conflict")
			return httputil.FailConflict(c, err.Error())
		case ingestsvc.ErrInvalidEventType:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [ApproveEvent] approval gate: eventType required")
			return httputil.FailBadRequest(c, "eventType is required before approval")
		default:
			log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [ApproveEvent] internal error")
			return httputil.FailInternal(c, "failed to approve event")
		}
	}

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✅ [ApproveEvent] event approved")

	return httputil.Ok(c, eventDetail, "event approved successfully")
}

// RejectEvent godoc
// @Summary      Reject pending event
// @Description  Rejects a pending event and removes it from the approval queue.
// @Tags         ingest-management
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        eventId       path      string  true  "Event ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      409  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/{eventId}/reject [post]
func (ctrl *EventManagementController) RejectEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventManagementController.RejectEvent", "ingestapi", "RejectEvent")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✏️ [RejectEvent] rejecting event")

	if err := ctrl.service.RejectEvent(ctx, tenantId, orgId, eventId, callerUserId); err != nil {
		switch err {
		case ingestsvc.ErrEventNotFound:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [RejectEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		case ingestsvc.ErrEventAlreadyApproved:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [RejectEvent] event already approved")
			return httputil.FailConflict(c, "event already approved")
		default:
			log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [RejectEvent] internal error")
			return httputil.FailInternal(c, "failed to reject event")
		}
	}

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✅ [RejectEvent] event rejected")

	return httputil.MessageOK(c, "event rejected successfully")
}

// DeletePendingEvent godoc
// @Summary      Delete pending event
// @Description  Permanently deletes a pending event. Cannot delete already-approved events.
// @Tags         ingest-management
// @Security     BearerAuth
// @Produce      json
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        eventId       path      string  true  "Event ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Failure      409  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/{eventId} [delete]
func (ctrl *EventManagementController) DeletePendingEvent(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "EventManagementController.DeletePendingEvent", "ingestapi", "DeletePendingEvent")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	eventId := c.Params("eventId")

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("🗑️ [DeletePendingEvent] deleting event")

	if err := ctrl.service.DeletePendingEvent(ctx, tenantId, orgId, eventId, callerUserId); err != nil {
		switch err {
		case ingestsvc.ErrEventNotFound:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [DeletePendingEvent] event not found")
			return httputil.FailNotFound(c, "event not found")
		case ingestsvc.ErrEventAlreadyApproved:
			log.Warn().Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [DeletePendingEvent] event already approved")
			return httputil.FailConflict(c, "event already approved")
		default:
			log.Error().Err(err).Str("orgId", orgId).Str("eventId", eventId).Msg("❌ [DeletePendingEvent] internal error")
			return httputil.FailInternal(c, "failed to delete event")
		}
	}

	log.Info().
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("callerUserId", callerUserId).
		Msg("✅ [DeletePendingEvent] event deleted")

	return httputil.MessageOK(c, "event deleted successfully")
}
