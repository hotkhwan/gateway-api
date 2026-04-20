// controllers/ingestapi/ingest.go
package ingestapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/adapters/alertdispatcher"
	"github.com/hotkhwan/gateway-api/internal/services/bindingsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ingestSvcI is the service subset used by IngestController.
// *ingestsvc.IngestService satisfies this interface.
type ingestSvcI interface {
	Ingest(ctx context.Context, orgId, sourceFamily, sourceIp, contentType string, body []byte) (*ingestsvc.IngestResult, error)
}

// alertDispatcherI is the dispatcher subset used by IngestController.
// *alertdispatcher.Dispatcher satisfies this interface.
type alertDispatcherI interface {
	Dispatch(alert alertdispatcher.FastAlertEnvelope) bool
}

// alertDetectorI is the detector subset used by IngestController.
// *alertdetectorsvc.AlertDetector satisfies this interface.
type alertDetectorI interface {
	HasAlert(payload map[string]any) bool
	Extract(payload map[string]any) map[string]any
}

// realtimeBindingGetter is the binding-service subset used for Path A realtime dispatch.
// *bindingsvc.BindingService satisfies this interface.
type realtimeBindingGetter interface {
	GetRealtimeBindings(ctx context.Context, workspaceId string) ([]ingestmod.TemplateDeliveryBinding, bool)
}

type IngestController struct {
	service    ingestSvcI
	alertDisp  alertDispatcherI       // nil = fast alert disabled
	alertDet   alertDetectorI         // nil = fast alert disabled
	bindingSvc realtimeBindingGetter  // nil = realtime binding dispatch disabled
}

func NewIngestController(svc ingestSvcI) *IngestController {
	if svc == nil {
		panic("IngestController: service required")
	}
	return &IngestController{service: svc}
}

// SetAlertDispatcher wires the bounded alert dispatcher and detector.
// Call from container after construction.
func (ctrl *IngestController) SetAlertDispatcher(d alertDispatcherI, det alertDetectorI) {
	ctrl.alertDisp = d
	ctrl.alertDet = det
}

// SetBindingService wires the binding service for realtime binding-based dispatch.
// Call from container after construction.
func (ctrl *IngestController) SetBindingService(svc realtimeBindingGetter) {
	ctrl.bindingSvc = svc
}

// Ingest godoc
// @Summary      Ingest raw event
// @Description  POST event payload from device or 3rd-party. No JWT required. Rate limited per org + IP.
// @Tags         ingest
// @Accept       json
// @Produce      json
// @Param        orgId  path      string  true  "Organization ID"
// @Success      202    {object}  gmod.SuccessDataResponse
// @Failure      400    {object}  gmod.ApiErrorResponse
// @Failure      404    {object}  gmod.ApiErrorResponse
// @Failure      413    {object}  gmod.ApiErrorResponse
// @Failure      423    {object}  gmod.ApiErrorResponse
// @Failure      429    {object}  gmod.ApiErrorResponse
// @Failure      500    {object}  gmod.ApiErrorResponse
// @Router       /events/{orgId}/{sourceFamily} [post]
func (ctrl *IngestController) Ingest(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "IngestController.Ingest", "ingestapi", "Ingest")
	defer end()

	orgId := strings.TrimSpace(c.Params("orgId"))
	if orgId == "" {
		log.Warn().Msg("❌ [Ingest] missing orgId")
		return httputil.FailBadRequest(c, "orgId is required")
	}

	sourceFamily := strings.TrimSpace(c.Params("sourceFamily"))
	if sourceFamily == "" {
		log.Warn().Msg("❌ [Ingest] missing sourceFamily")
		return httputil.FailBadRequest(c, "sourceFamily is required")
	}

	body := c.Body()
	sourceIp := c.IP()
	contentType := c.Get("Content-Type", "application/json")

	// Debug — hot path: every request logs at Debug to avoid production noise
	log.Debug().
		Str("orgId", orgId).
		Str("sourceFamily", sourceFamily).
		Str("sourceIp", sourceIp).
		Str("contentType", contentType).
		Int("bodySize", len(body)).
		Msg("📥 [Ingest] incoming event")

	// Path A — Fast Alert (provisional, non-blocking)
	// Runs before Path B (Kafka publish) so the alert reaches UI before canonical storage.
	// Uses traceutil.DetachWithParent so the dispatcher goroutine outlives the request.
	// Parse payload once and reuse for both alert detection and binding dispatch.
	now := time.Now().UTC().Format(time.RFC3339)
	var parsedPayload map[string]any
	_ = json.Unmarshal(body, &parsedPayload) // best-effort; nil on failure

	if ctrl.alertDisp != nil && ctrl.alertDet != nil && parsedPayload != nil {
		if ctrl.alertDet.HasAlert(parsedPayload) {
			alertCtx := traceutil.DetachWithParent(ctx)
			_ = alertCtx // ctx carried implicitly via closure below
			alert := alertdispatcher.FastAlertEnvelope{
				EventID:      "", // will be populated by Path B; UI reconciles later
				WorkspaceID:  orgId,
				SourceFamily: sourceFamily,
				OccurredAt:   now,
				Provisional:  true,
				Canonical:    false,
				AlertFields:  ctrl.alertDet.Extract(parsedPayload),
			}
			ctrl.alertDisp.Dispatch(alert)
		}
	}

	// Realtime binding dispatch — check Redis cache for workspace bindings.
	// For each enabled realtime binding whose matchFields matches the payload, fire a provisional alert.
	// Non-blocking: uses the same alertdispatcher to avoid delaying the main Kafka path.
	if ctrl.bindingSvc != nil && ctrl.alertDisp != nil && parsedPayload != nil {
		if bindings, ok := ctrl.bindingSvc.GetRealtimeBindings(ctx, orgId); ok {
			for _, b := range bindings {
				if bindingsvc.MatchesFields(parsedPayload, b.MatchFields) {
					ctrl.alertDisp.Dispatch(alertdispatcher.FastAlertEnvelope{
						EventID:      "", // populated by Path B; UI reconciles via canonical notify
						WorkspaceID:  orgId,
						SourceFamily: sourceFamily,
						OccurredAt:   now,
						Provisional:  true,
						Canonical:    false,
						AlertFields:  parsedPayload,
					})
				}
			}
		}
	}

	result, err := ctrl.service.Ingest(ctx, orgId, sourceFamily, sourceIp, contentType, body)
	if err != nil {
		switch {
		case errors.Is(err, ingestsvc.ErrSourceFamilyLocked):
			log.Info().Str("orgId", orgId).Str("sourceFamily", sourceFamily).Msg("[Ingest] sourceFamily locked")
			return httputil.Fail(c, fiber.StatusLocked, "SOURCE_FAMILY_LOCKED", "sourceFamily is not enabled")
		case errors.Is(err, ingestsvc.ErrSourceFamilyComingSoon):
			log.Info().Str("orgId", orgId).Str("sourceFamily", sourceFamily).Msg("[Ingest] sourceFamily coming soon")
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"code":    "ACCEPTED",
				"message": "sourceFamily is coming soon",
				"status":  true,
			})
		case errors.Is(err, ingestsvc.ErrOrgNotFound):
			log.Warn().Str("orgId", orgId).Msg("[Ingest] org not found")
			return httputil.FailNotFound(c, "organization not found")
		case errors.Is(err, ingestsvc.ErrRateLimited):
			log.Warn().Str("orgId", orgId).Str("sourceIp", sourceIp).Msg("[Ingest] rate limited")
			return httputil.FailTooMany(c, "rate limit exceeded")
		case errors.Is(err, ingestsvc.ErrPayloadTooLarge):
			log.Warn().Str("orgId", orgId).Int("bodySize", len(body)).Msg("[Ingest] payload too large")
			return httputil.Fail(c, fiber.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "payload exceeds limit")
		case errors.Is(err, ingestsvc.ErrEmptyBody):
			log.Warn().Str("orgId", orgId).Msg("[Ingest] empty body")
			return httputil.FailBadRequest(c, "request body is empty")
		case errors.Is(err, ingestsvc.ErrTemplateMismatch):
			log.Warn().Str("orgId", orgId).Msg("[Ingest] no matching template for approved device")
			return httputil.Fail(c, fiber.StatusUnprocessableEntity, "TEMPLATE_MISMATCH", "no matching template found, please configure a mapping template for this device")
		default:
			log.Error().Err(err).Str("orgId", orgId).Msg("[Ingest] internal error")
			return httputil.FailInternal(c, "internal server error")
		}
	}

	// Device pending lock — device already has an unresolved pending event
	if result.Locked {
		log.Warn().
			Str("orgId", orgId).
			Str("deviceKey", result.DeviceKey).
			Str("pendingEventId", result.EventId).
			Msg("⚠️ [Ingest] device pending locked")
		return httputil.Fail(c, fiber.StatusLocked, "DEVICE_PENDING_LOCKED", result.LockMessage, fiber.Map{
			"deviceKey":      result.DeviceKey,
			"pendingEventId": result.EventId,
		})
	}

	// Event queued in event_management — awaiting admin review + mapping template
	if result.Pending {
		log.Debug().
			Str("orgId", orgId).
			Str("eventId", result.EventId).
			Msg("📋 [Ingest] event queued for review")
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"code":    "PENDING_REVIEW",
			"message": "event queued for review",
			"status":  true,
			"details": fiber.Map{
				"eventId":    result.EventId,
				"receivedAt": result.ReceivedAt,
			},
		})
	}

	// Debug — hot path: every accepted event logs at Debug
	log.Debug().
		Str("orgId", orgId).
		Str("eventId", result.EventId).
		Msg("✅ [Ingest] event accepted")

	return httputil.Accepted(c, fiber.Map{
		"eventId":    result.EventId,
		"receivedAt": result.ReceivedAt,
	}, "event accepted")
}
