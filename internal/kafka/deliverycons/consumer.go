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

	templateIdSource := "meta"
	if event.Meta.TemplateId == "" {
		var rawTop struct {
			TemplateId string `json:"templateId"`
		}
		if err := json.Unmarshal(m.Value, &rawTop); err == nil && rawTop.TemplateId != "" {
			event.Meta.TemplateId = rawTop.TemplateId
			templateIdSource = "rootJson"
		}
	}
	if event.Meta.TemplateId == "" {
		for _, h := range m.Headers {
			if h.Key == "templateId" && len(h.Value) > 0 {
				event.Meta.TemplateId = string(h.Value)
				templateIdSource = "header"
				break
			}
		}
	}

	log.Debug().
		Str("eventId", event.EventId).
		Str("templateId", event.Meta.TemplateId).
		Str("templateIdSource", templateIdSource).
		Str("topic", m.Topic).
		Msg("[delivery] templateId resolved (defensive)")

	dispatchToTargets(ctx, &event, deps)
}
