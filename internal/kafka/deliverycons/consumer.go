// internal/kafka/deliverycons/consumer.go
package deliverycons

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/segmentio/kafka-go"
)

// StartDeliveryConsumer consumes gw.events.normalized.v1 directly (Wide slice
// post-unification). Each Kafka message is decoded as eventschema.NormalizedEvent,
// converted to the internal ingestmod.NormalizedEvent shape via FromEventSchema,
// and handed to dispatchToTargets along with the original raw bytes. The raw
// bytes are used verbatim as webhook body so receivers see exactly what Kafka
// carries — no lossy bridging, no hydration, no canonical re-synthesis.
//
// Consumer group: gateway-delivery-v2-group (LastOffset — never replay history).
//
// DELIVERY_V2_ENABLED env var still exists for operational reversibility:
//   - "true"  (default after r2) → dispatch authoritatively
//   - "false"                    → dry-run: decode + log intended targets, no dispatch
func StartDeliveryConsumer(ctx context.Context, deps ConsumerDeps) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_GW_EVENTS_NORMALIZED", "gw.events.normalized.v1")
	groupID := "gateway-delivery-v2-group"

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
		StartOffset:    kafka.LastOffset, // Contract §5.3 — never replay history
	})
	defer func() { _ = reader.Close() }()

	dryRun := os.Getenv("DELIVERY_V2_ENABLED") == "false"
	log.Info().Bool("dryRun", dryRun).Msg("delivery consumer started (v2, reads gw.events.normalized.v1)")

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

		handleCanonicalEvent(ctx, m, deps, dryRun)
		_ = reader.CommitMessages(ctx, m)
	}
}

// handleCanonicalEvent decodes a gw.events.normalized.v1 message and
// dispatches to the template's delivery targets. The original raw bytes are
// retained and passed through as the webhook body (contract §5.5 — Phase-E).
func handleCanonicalEvent(ctx context.Context, m kafka.Message, deps ConsumerDeps, dryRun bool) {
	log := deps.Logger

	var src eventschema.NormalizedEvent
	if err := json.Unmarshal(m.Value, &src); err != nil {
		log.Error().
			Err(err).
			Int("partition", m.Partition).
			Int64("offset", m.Offset).
			Msg("[delivery] decode eventschema failed — skipping")
		return
	}

	event := FromEventSchema(&src)
	if event == nil {
		return
	}

	if dryRun {
		log.Info().
			Str("eventId", event.EventId).
			Str("templateId", event.Meta.TemplateId).
			Str("workspaceId", event.Source.WorkspaceId).
			Msg("[delivery-v2 dry-run] would dispatch")
		return
	}

	dispatchToTargets(ctx, event, m.Value, deps)
}
