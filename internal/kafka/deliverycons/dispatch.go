// internal/kafka/deliverycons/dispatch.go
package deliverycons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/discordgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/linegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/telegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/webhookgw"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/mongo"
)

// dispatchToTargets loads the mapping template for the event's templateId/orgId,
// evaluates classificationRules to set eventClass/eventSeverity,
// then evaluates per-target filters (payload + eventClasses + eventSeverities)
// and dispatches to each matching enabled target.
// Each target failure is recorded individually in the DLQ (messageId: {eventId}:deliver:{targetId}).
func dispatchToTargets(ctx context.Context, event *ingestmod.NormalizedEvent, deps ConsumerDeps) {
	log := deps.Logger.With().
		Str("eventId", event.EventId).
		Str("orgId", event.Source.OrgId).
		Logger()

	templateId := event.Meta.TemplateId
	if templateId == "" {
		log.Debug().Msg("[delivery] no templateId — skipping dispatch")
		return
	}

	tmpl, err := deps.TemplateRepo.FindById(ctx, event.Source.OrgId, templateId)
	if err != nil {
		log.Warn().Err(err).Str("templateId", templateId).Msg("[delivery] template not found — skipping dispatch")
		return
	}

	// Step 1: Evaluate classificationRules to set eventClass/eventSeverity
	// Always apply defaults (unknown/none) even when no rules exist
	applyClassificationRules(event, tmpl.ClassificationRules)
	log.Debug().
		Str("eventClass", event.EventClass).
		Str("eventSeverity", event.EventSeverity).
		Msg("[delivery] classification applied")

	if len(tmpl.DeliveryTargets) == 0 {
		log.Debug().Str("templateId", templateId).Msg("[delivery] no delivery targets configured")
		return
	}

	payload, _ := json.Marshal(event)

	for _, tdt := range tmpl.DeliveryTargets {
		// Step 2: Evaluate payload filter
		if !matchesFilter(event.Payload, tdt.Filter) {
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
		target, err := deps.TargetRepo.FindByIDAndOrg(ctx, tdt.TargetId, event.TenantId, event.Source.OrgId)
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
		msgPayload, err := buildMessagePayload(event, tmpl, target.Type, target.Config, messageTemplateKey)
		if err != nil {
			return err
		}
		return client.Send(ctx, event, msgPayload)

	case authzmod.TargetTypeDiscord:
		client := discordgw.NewClient(target.Config)
		msgPayload, err := buildMessagePayload(event, tmpl, target.Type, target.Config, messageTemplateKey)
		if err != nil {
			return err
		}
		return client.Send(ctx, event, msgPayload)

	case authzmod.TargetTypeTelegram:
		client := telegw.NewClient(target.Config)
		msgPayload, err := buildMessagePayload(event, tmpl, target.Type, target.Config, messageTemplateKey)
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
func buildMessagePayload(
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

	if mt == nil {
		// No template configured — fall back to raw JSON payload
		return json.Marshal(event)
	}

	title, body, err := renderMessage(mt, event)
	if err != nil {
		return nil, fmt.Errorf("render message: %w", err)
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
	if mt.Extras != nil {
		msg["extras"] = mt.Extras
	}
	return json.Marshal(msg)
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
		OrgId:               event.Source.OrgId,
		TemplateId:          event.Meta.TemplateId,
		Topic:               "normalized.events",
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
		"orgId":      event.Source.OrgId,
		"tenantId":   event.TenantId,
		"targetId":   targetId,
		"targetType": targetType,
		"status":     status,
		"error":      errMsg,
	})
	headers := map[string]string{
		"eventId":  event.EventId,
		"orgId":    event.Source.OrgId,
		"status":   status,
		"targetId": targetId,
	}
	go func() {
		_ = config.SendToKafkaWithCtx(context.Background(), topic, event.Source.OrgId, payload, headers)
	}()
}
