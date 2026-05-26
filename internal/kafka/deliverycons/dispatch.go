// internal/kafka/deliverycons/dispatch.go
package deliverycons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/discordgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/linegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/telegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/webhookgw"
	"github.com/hotkhwan/gateway-api/internal/services/classification"
	"github.com/hotkhwan/gateway-api/internal/services/templatematcher"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/mongo"
)

// passesTemplateDeliveryGate applies the template-level delivery gate:
// Enabled master toggle (plan decision D4) followed by DeliveryMatchAll /
// DeliveryMatchAny (plan decisions D2, D6). Pure — no I/O — so it can be
// unit-tested in isolation. Logs a structured skip record and increments the
// corresponding in-process counter when the gate blocks dispatch.
func passesTemplateDeliveryGate(
	tmpl *ingestmod.MappingTemplate,
	event *ingestmod.NormalizedEvent,
	log zerolog.Logger,
) bool {
	if !tmpl.Enabled {
		deliverySkipDisabled.Add(1)
		log.Info().
			Str("templateId", tmpl.TemplateId).
			Str("workspaceId", event.Source.WorkspaceId).
			Str("skipReason", "disabled").
			Msg("[delivery] template disabled — skip all targets")
		return false
	}
	if len(tmpl.DeliveryMatchAll) == 0 && len(tmpl.DeliveryMatchAny) == 0 {
		return true
	}
	bag := buildDeliveryMatchBag(event)
	if templatematcher.Evaluate(tmpl.DeliveryMatchAll, tmpl.DeliveryMatchAny, bag) {
		return true
	}
	deliverySkipDeliveryRuleMiss.Add(1)
	log.Info().
		Str("templateId", tmpl.TemplateId).
		Str("workspaceId", event.Source.WorkspaceId).
		Str("skipReason", "delivery_rule_miss").
		Msg("[delivery] template delivery rule not matched — skip all targets")
	return false
}

// dispatchToTargets loads the mapping template for the event's templateId/orgId,
// evaluates classificationRules to set eventClass/eventSeverity,
// then evaluates per-target filters (payload + eventClasses + eventSeverities)
// and dispatches to each matching enabled target.
//
// rawCanonical is the original gw.events.normalized.v1 wire payload — used
// verbatim as the webhook body so receivers see exactly what Kafka carries.
// Each target failure is recorded individually in the DLQ.
func dispatchToTargets(ctx context.Context, event *ingestmod.NormalizedEvent, rawCanonical []byte, deps ConsumerDeps) {
	log := deps.Logger.With().
		Str("eventId", event.EventId).
		Str("orgId", event.Source.WorkspaceId).
		Logger()

	templateId := event.Meta.TemplateId
	if templateId == "" {
		log.Debug().Msg("[delivery] no templateId — skipping dispatch")
		return
	}

	// Load template first; the template-level gate needs Enabled and the
	// delivery rules. Loading is a cheap Mongo read (with Redis cache).
	tmpl, err := deps.TemplateRepo.FindById(ctx, event.Source.WorkspaceId, templateId)
	if err != nil {
		log.Warn().Err(err).Str("templateId", templateId).Msg("[delivery] template not found — skipping dispatch")
		return
	}

	// Template-level gate: enforces Template.Enabled + DeliveryMatchAll/Any.
	// Placed before classification so skipped events incur zero enrichment work.
	if !passesTemplateDeliveryGate(tmpl, event, log) {
		return
	}

	// Step 1: Evaluate classificationRules to set eventClass/eventSeverity.
	// Always apply defaults (unknown/none) even when no rules exist.
	// Idempotent: when the producer-side normalizer already classified, this
	// re-application is a no-op for first-match-wins (the shared evaluator
	// guarantees both sides resolve paths identically — see
	// internal/services/classification and contract §5A.3).
	classification.Apply(event, tmpl.ClassificationRules)
	log.Debug().
		Str("eventClass", event.EventClass).
		Str("eventSeverity", event.EventSeverity).
		Msg("[delivery] classification applied")

	if len(tmpl.DeliveryTargets) == 0 {
		log.Debug().Str("templateId", templateId).Msg("[delivery] no delivery targets configured")
		return
	}

	// Webhook body = raw gw.events.normalized.v1 wire payload (contract §5.5).
	// Fallback to re-marshal of the ingestmod shape if raw is empty (defensive
	// against callers that didn't pass through the original bytes).
	payload := rawCanonical
	if len(payload) == 0 {
		payload, _ = json.Marshal(event)
	}

	for _, tdt := range tmpl.DeliveryTargets {
		// Step 2: Evaluate payload filter
		if !classification.Matches(event.Payload, tdt.Filter) {
			log.Debug().
				Str("targetId", tdt.TargetId).
				Msg("[delivery] payload filter not matched — skip")
			continue
		}

		// Step 3: Evaluate eventClasses whitelist
		if !matchesEventClasses(event.EventClass, tdt.EventClasses) {
			log.Debug().
				Str("targetId", tdt.TargetId).
				Str("eventClass", event.EventClass).
				Msg("[delivery] eventClass not matched — skip")
			continue
		}

		// Step 4: Evaluate eventSeverities whitelist
		if !matchesEventSeverities(event.EventSeverity, tdt.EventSeverities) {
			log.Debug().
				Str("targetId", tdt.TargetId).
				Str("eventSeverity", event.EventSeverity).
				Msg("[delivery] eventSeverity not matched — skip")
			continue
		}

		// Load target config
		// Scope by workspace only — event.TenantId may be klynxOrgId from upstream
		// republish while the target was stored with the original tenant string.
		// Workspace is 1-to-1 with tenant, so targetId + workspaceId is sufficient.
		target, err := deps.TargetRepo.FindByIDAndWorkspace(ctx, tdt.TargetId, event.Source.WorkspaceId)
		if err != nil {
			log.Warn().
				Str("targetId", tdt.TargetId).
				Err(err).
				Msg("[delivery] target not found — skip")
			continue
		}

		if !target.Enabled {
			log.Debug().Str("targetId", tdt.TargetId).Msg("[delivery] target disabled — skip")
			continue
		}

		// Step 5: Dispatch (webhook = raw JSON, message channels = render by messageTemplateKey)
		dispErr := sendToTarget(ctx, event, target, tmpl, tdt.MessageTemplateKey, payload)
		if dispErr == nil {
			// Resolve any existing DLQ record for this target
			if deps.DLQRepo != nil {
				msgId := fmt.Sprintf("%s:deliver:%s", event.EventId, tdt.TargetId)
				_ = deps.DLQRepo.UpdateStatus(ctx, event.TenantId, msgId, "resolved", 0)
			}
			log.Debug().
				Str("targetId", tdt.TargetId).
				Str("type", target.Type).
				Msg("[delivery] dispatched successfully")
			publishDeliveryStatus(ctx, event, tdt.TargetId, target.Type, "success", "")
			continue
		}

		log.Error().
			Str("targetId", tdt.TargetId).
			Str("type", target.Type).
			Err(dispErr).
			Msg("[delivery] dispatch failed — inserting DLQ")

		publishDeliveryStatus(ctx, event, tdt.TargetId, target.Type, "failed", dispErr.Error())
		insertDeliveryDLQ(ctx, deps, event, tmpl, tdt.TargetId, payload, dispErr.Error())
	}
}

// sendToTarget dispatches a normalized event to a single delivery target.
// For message channels, messageTemplateKey is used to select the specific template.
func sendToTarget(
	ctx context.Context,
	event *ingestmod.NormalizedEvent,
	target *authzmod.DeliveryTarget,
	tmpl *ingestmod.MappingTemplate,
	messageTemplateKey string,
	rawPayload []byte,
) error {
	switch target.Type {
	case authzmod.TargetTypeWebhook:
		client := webhookgw.NewClient(target.Config)
		return client.Send(ctx, event, rawPayload)

	case authzmod.TargetTypeLine:
		client := linegw.NewClient(target.Config)
		msgPayload, err := buildMessagePayload(ctx, event, tmpl, target.Type, target.Config, messageTemplateKey)
		if err != nil {
			return err
		}
		return client.Send(ctx, event, msgPayload)

	case authzmod.TargetTypeDiscord:
		client := discordgw.NewClient(target.Config)
		msgPayload, err := buildMessagePayload(ctx, event, tmpl, target.Type, target.Config, messageTemplateKey)
		if err != nil {
			return err
		}
		return client.Send(ctx, event, msgPayload)

	case authzmod.TargetTypeTelegram:
		client := telegw.NewClient(target.Config)
		msgPayload, err := buildMessagePayload(ctx, event, tmpl, target.Type, target.Config, messageTemplateKey)
		if err != nil {
			return err
		}
		return client.Send(ctx, event, msgPayload)

	default:
		return fmt.Errorf("unknown target type: %s", target.Type)
	}
}

// buildMessagePayload renders the message template for notification channels and returns
// a JSON payload shaped for that channel's gateway.
// If messageTemplateKey is set, it selects the template by key first; otherwise falls back
// to the existing channelType+locale selection.
//
// For LINE targets, the envelope additionally carries a "flex" object — a
// LINE Flex Bubble built from the template extras (tag, action buttons, etc.)
// and the first image binaryRef (presigned via S3). linegw.Client prefers
// "flex" over "text" when present.
func buildMessagePayload(
	ctx context.Context,
	event *ingestmod.NormalizedEvent,
	tmpl *ingestmod.MappingTemplate,
	channelType string,
	cfg authzmod.TargetConfig,
	messageTemplateKey string,
) ([]byte, error) {
	locale := tmpl.DefaultLocale
	if locale == "" {
		locale = "en"
	}

	var mt *ingestmod.MessageTemplate

	// Try to find by messageTemplateKey first
	if messageTemplateKey != "" {
		mt = selectTemplateByKey(tmpl.MessageTemplates, messageTemplateKey)
	}

	// Fallback to channelType+locale selection
	if mt == nil {
		mt = selectTemplate(tmpl.MessageTemplates, channelType, locale, tmpl.DefaultLocale)
	}

	var title, body string
	var extras map[string]string
	if mt != nil {
		var err error
		title, body, err = renderMessage(mt, event)
		if err != nil {
			return nil, fmt.Errorf("render message: %w", err)
		}
		extras = mt.Extras
	}

	// Build a generic message envelope that each gateway interprets
	msg := map[string]any{
		"eventId":       event.EventId,
		"eventType":     event.EventType,
		"eventClass":    event.EventClass,
		"eventSeverity": event.EventSeverity,
		"title":         title,
		"body":          body,
	}
	if extras != nil {
		msg["extras"] = extras
	}

	// Default location enrichment — so every channel that wants lat/lng can use
	// it without re-deriving from the raw event.
	if event.Location.Lat != 0 || event.Location.Lng != 0 {
		msg["lat"] = event.Location.Lat
		msg["lng"] = event.Location.Lng
	}

	// First image binaryRef as a presigned URL — used by Telegram (sendPhoto)
	// and any future channel that wants to render an image without going back
	// to the event itself.
	if url := firstImagePresignedURL(ctx, event); url != "" {
		msg["imageUrl"] = url
	}

	// Event-details deep link — template extras win, otherwise fall back to the
	// WEB_BASE_URL default so every channel can render a "View details" action
	// without per-template configuration.
	eventDetailsURL := resolveEventDetailsURL(event, extras)
	if eventDetailsURL != "" {
		msg["eventDetailsUrl"] = eventDetailsURL
	}

	if channelType == authzmod.TargetTypeLine && mt != nil {
		if flex := buildFlexCard(ctx, event, tmpl, mt, title, body, eventDetailsURL); flex != nil {
			msg["flex"] = flex
		}
	}
	return json.Marshal(msg)
}

// resolveEventDetailsURL chooses the deep link rendered into delivery messages.
// Precedence: template extras["eventDetailsUrl"] (rendered with event data) →
// WEB_BASE_URL/events/{eventId} default. Returns "" when neither is set.
func resolveEventDetailsURL(event *ingestmod.NormalizedEvent, extras map[string]string) string {
	if raw := strings.TrimSpace(extras["eventDetailsUrl"]); raw != "" {
		if rendered, err := renderText(raw, renderContext(event)); err == nil {
			return strings.TrimSpace(rendered)
		}
		return raw
	}
	return config.EventDetailsURL(event.EventId)
}

// firstImagePresignedURL returns a presigned GET URL for the first image
// binaryRef on the event, or "" when there is no image or presigning fails.
func firstImagePresignedURL(ctx context.Context, event *ingestmod.NormalizedEvent) string {
	for _, ref := range event.BinaryRefs {
		if ref.Kind != "image" && !strings.HasPrefix(ref.ContentType, "image/") {
			continue
		}
		url, err := config.PresignS3GetURL(ctx, ref.Bucket, ref.ObjectId)
		if err != nil {
			continue
		}
		return url
	}
	return ""
}

// insertDeliveryDLQ records a per-target delivery failure in the DLQ.
// messageId format: {eventId}:deliver:{targetId}
func insertDeliveryDLQ(
	ctx context.Context,
	deps ConsumerDeps,
	event *ingestmod.NormalizedEvent,
	tmpl *ingestmod.MappingTemplate,
	targetId string,
	payload []byte,
	reason string,
) {
	if deps.DLQRepo == nil {
		return
	}

	dlqCfg := ingestmod.DLQConfig{Enabled: true, MaxRetries: 3, RetryTimeoutSeconds: 60}
	if tmpl != nil {
		if !tmpl.DLQ.Enabled {
			return
		}
		dlqCfg = tmpl.DLQ
	}

	now := time.Now().UTC()
	messageId := fmt.Sprintf("%s:deliver:%s", event.EventId, targetId)

	var rawPayload map[string]any
	_ = json.Unmarshal(payload, &rawPayload)

	msg := &ingestmod.DLQMessage{
		MessageId:           messageId,
		EventId:             event.EventId,
		TenantId:            event.TenantId,
		WorkspaceId:         event.Source.WorkspaceId,
		TemplateId:          event.Meta.TemplateId,
		Topic:               "gw.events.normalized.v1",
		Stage:               "deliver",
		Reason:              reason,
		Payload:             rawPayload,
		RetryCount:          0,
		MaxRetries:          dlqCfg.MaxRetries,
		RetryTimeoutSeconds: dlqCfg.RetryTimeoutSeconds,
		Status:              "pending",
		LastErrorAt:         now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := deps.DLQRepo.Insert(ctx, msg); err != nil {
		if !isMongoDupKey(err) {
			deps.Logger.Warn().
				Str("eventId", event.EventId).
				Str("targetId", targetId).
				Str("messageId", messageId).
				Err(err).
				Msg("[delivery] DLQ insert failed")
		}
	}
}

func isMongoDupKey(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, wErr := range we.WriteErrors {
			if wErr.Code == 11000 {
				return true
			}
		}
	}
	return false
}

// publishDeliveryStatus fires a non-blocking Kafka publish to gw.delivery.status.v1.
// Non-fatal: errors are silently discarded.
func publishDeliveryStatus(ctx context.Context, event *ingestmod.NormalizedEvent, targetId, targetType, status, errMsg string) {
	topic := config.TopicEnv("KAFKA_TOPIC_GW_DELIVERY_STATUS", "gw.delivery.status.v1")
	payload, _ := json.Marshal(map[string]any{
		"eventId":    event.EventId,
		"orgId":      event.Source.WorkspaceId,
		"tenantId":   event.TenantId,
		"targetId":   targetId,
		"targetType": targetType,
		"status":     status,
		"error":      errMsg,
	})
	headers := map[string]string{
		"eventId":  event.EventId,
		"orgId":    event.Source.WorkspaceId,
		"status":   status,
		"targetId": targetId,
	}
	go func() {
		_ = config.SendToKafkaWithCtx(context.Background(), topic, event.Source.WorkspaceId, payload, headers)
	}()
}
