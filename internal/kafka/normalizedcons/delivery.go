// internal/kafka/normalizedcons/delivery.go
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

// StartDeliveryConsumer consumes normalized.events and delivers to webhook targets.
func StartDeliveryConsumer(broker, topic string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "normalizedcons", "StartDeliveryConsumer")

	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("KAFKA_TOPIC_NORMALIZED_EVENTS"))
		if topic == "" {
			topic = "normalized.events"
		}
	}

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
		Msg("starting delivery consumer")

	kafka.StartConsumerWithHeaders(
		broker,
		topic,
		groupID,
		func(msg deliverysvc.NormalizedEvent, headers map[string]string) error {
			log.Debug().
				Str("eventId", msg.EventId).
				Str("tenantId", msg.TenantId).
				Str("orgId", msg.WorkspaceId).
				Str("eventType", msg.EventType).
				Msg("normalized event received for delivery")
			return deliverysvc.HandleNormalizedEvent(ctx, msg)
		},
	)
}
