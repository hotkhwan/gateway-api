// internal/kafka/deliverycons/consumer.go
package deliverycons

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/segmentio/kafka-go"
)

type resolvedFieldSources struct {
	TemplateID  string
	WorkspaceID string
	TenantID    string
}

type rawNormalizedEvent struct {
	TemplateID  string `json:"templateId"`
	TenantID    string `json:"tenantId"`
	WorkspaceID string `json:"workspaceId"`
	OrgID       string `json:"orgId"`
	Source      struct {
		WorkspaceID string `json:"workspaceId"`
		OrgID       string `json:"orgId"`
	} `json:"source"`
}

// StartDeliveryConsumer consumes normalized.events, loads the matching template,
// and dispatches each event to its configured delivery targets.
// Consumer group: gateway-delivery-group
func StartDeliveryConsumer(ctx context.Context, deps ConsumerDeps) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
	groupID := "gateway-delivery-group"

	log := deps.Logger.With().
		Str("topic", topic).
		Str("group", groupID).
		Logger()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1e3,
		MaxBytes:       10e6,
		MaxWait:        10 * time.Second,
		CommitInterval: 0,
	})
	defer func() { _ = reader.Close() }()

	log.Info().Msg("delivery consumer started")

	for {
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info().Msg("delivery consumer shutting down")
				return
			}
			log.Error().Err(err).Msg("[delivery] fetch error")
			time.Sleep(time.Second)
			continue
		}

		handleNormalizedEvent(ctx, m, deps)
		_ = reader.CommitMessages(ctx, m)
	}
}

// handleNormalizedEvent decodes a normalized.events message and dispatches it to targets.
//
// templateId resolution (in order of precedence — first non-empty wins):
//  1. event.Meta.TemplateId        (canonical ingestmod.NormalizedEvent shape)
//  2. raw JSON top-level templateId (eventschema.NormalizedEvent / bridge shape)
//  3. Kafka header "templateId"     (set by upstream producer)
//
// Defensive against schema drift when delivery consumer subscribes either
// normalized.events (Meta.TemplateId) or gw.events.normalized.v1 (root-level).
func handleNormalizedEvent(ctx context.Context, m kafka.Message, deps ConsumerDeps) {
	log := deps.Logger

	var event ingestmod.NormalizedEvent
	if err := json.Unmarshal(m.Value, &event); err != nil {
		log.Error().
			Err(err).
			Int("partition", m.Partition).
			Int64("offset", m.Offset).
			Msg("[delivery] decode failed — skipping")
		return
	}

	sources := resolveNormalizedEventFields(&event, m)

	log.Debug().
		Str("eventId", event.EventId).
		Str("templateId", event.Meta.TemplateId).
		Str("templateIdSource", sources.TemplateID).
		Str("workspaceId", event.Source.WorkspaceId).
		Str("workspaceIdSource", sources.WorkspaceID).
		Str("tenantId", event.TenantId).
		Str("tenantIdSource", sources.TenantID).
		Str("topic", m.Topic).
		Msg("[delivery] templateId resolved (defensive)")

	dispatchToTargets(ctx, &event, deps)
}

func resolveNormalizedEventFields(event *ingestmod.NormalizedEvent, m kafka.Message) resolvedFieldSources {
	sources := resolvedFieldSources{
		TemplateID:  "meta",
		WorkspaceID: "payload",
		TenantID:    "payload",
	}

	var raw rawNormalizedEvent
	_ = json.Unmarshal(m.Value, &raw)

	if event.Meta.TemplateId == "" {
		if raw.TemplateID != "" {
			event.Meta.TemplateId = raw.TemplateID
			sources.TemplateID = "rootJson"
		} else if v := headerValue(m.Headers, "templateId"); v != "" {
			event.Meta.TemplateId = v
			sources.TemplateID = "header"
		}
	}

	if event.Source.WorkspaceId == "" {
		// Resolve workspaceId from authoritative sources only.
		// orgId fields are NOT fallbacks for workspaceId — klynx-api's minimal republish
		// puts klynxOrgId at source.orgId / root orgId, which is a different entity.
		// Kafka header "workspaceId" is set explicitly by upstream publishers
		// (normalizedcons in gateway-api, handleNormalized in klynx-api) and is the
		// authoritative phibek workspaceId when the JSON payload omits it.
		headerWorkspaceID := headerValue(m.Headers, "workspaceId")
		switch {
		case raw.Source.WorkspaceID != "":
			event.Source.WorkspaceId = raw.Source.WorkspaceID
			sources.WorkspaceID = "sourceRootJson"
		case raw.WorkspaceID != "":
			event.Source.WorkspaceId = raw.WorkspaceID
			sources.WorkspaceID = "rootJson"
		case headerWorkspaceID != "":
			event.Source.WorkspaceId = headerWorkspaceID
			sources.WorkspaceID = "header"
		}
	}

	if event.TenantId == "" {
		if raw.TenantID != "" {
			event.TenantId = raw.TenantID
			sources.TenantID = "rootJson"
		} else if v := headerValue(m.Headers, "tenantId"); v != "" {
			event.TenantId = v
			sources.TenantID = "header"
		}
	}

	return sources
}

func headerValue(headers []kafka.Header, key string) string {
	for _, h := range headers {
		if h.Key == key && len(h.Value) > 0 {
			return string(h.Value)
		}
	}
	return ""
}
