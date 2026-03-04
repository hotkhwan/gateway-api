// internal/kafka/normalizedcons/consumer.go
package normalizedcons

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/deliverysvc"
)

// StartKafkaNormalizedEventConsumer consumes normalized events and delivers to targets
func StartKafkaNormalizedEventConsumer(broker, topic string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "normalizedcons", "StartKafkaNormalizedEventConsumer")

	// Use topic from parameter or env
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("KAFKA_TOPIC_NORMALIZED_EVENTS"))
		if topic == "" {
			topic = "normalized.events"
		}
	}

	// Generate groupID
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "normalized.consumer"
	}

	log.Debug().
		Str("broker", broker).
		Str("topic", topic).
		Str("group", groupID).
		Msg("🟢 Starting Kafka Normalized Event Consumer")

	// Start consumer using generic pattern
	kafka.StartConsumerWithHeaders(
		broker,
		topic,
		groupID,
		func(msg deliverysvc.NormalizedEvent, headers map[string]string) error {
			log.Debug().
				Str("eventId", msg.EventId).
				Str("tenantId", msg.TenantId).
				Str("orgId", msg.OrgId).
				Str("eventType", msg.EventType).
				Msg("📥 Normalized event received")

			// Convert to delivery format and handle
			return deliverysvc.HandleNormalizedEvent(ctx, msg)
		},
	)
}
