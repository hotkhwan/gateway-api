// internal/eventbridge/publisher.go
package eventbridge

import (
	"context"
	"os"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	kafkautil "github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// EventBridgePublisher publishes NormalizedEvents to downstream consumers.
// Used exclusively in the DEPLOYMENT_PROFILE=appliance topology, where
// phibek and klynx-api share the same Kafka broker.
//
// saasPublic profile uses deliveryOrchestrator (webhook) instead — do NOT use
// EventBridgePublisher in that profile.
type EventBridgePublisher interface {
	Publish(ctx context.Context, event eventschema.NormalizedEvent) error
}

// kafkaEventBridgePublisher publishes to events.normalized.v1.
type kafkaEventBridgePublisher struct {
	topic string
}

// NewKafkaEventBridgePublisher creates an EventBridgePublisher that writes to the
// gw.events.normalized.v1 Kafka topic.
// The topic name is read from KAFKA_TOPIC_EVENTS_NORMALIZED env var, defaulting
// to "gw.events.normalized.v1".
func NewKafkaEventBridgePublisher() EventBridgePublisher {
	topic := os.Getenv("KAFKA_TOPIC_EVENTS_NORMALIZED")
	if topic == "" {
		topic = "gw.events.normalized.v1"
	}
	return &kafkaEventBridgePublisher{topic: topic}
}

// Publish sends a NormalizedEvent to the EventBridge Kafka topic.
// Trace context is injected into Kafka message headers (not the event payload).
func (p *kafkaEventBridgePublisher) Publish(ctx context.Context, event eventschema.NormalizedEvent) error {
	headers := map[string]string{}
	traceutil.InjectHeaders(ctx, headers)

	// Forward templateId via header so delivery consumers can resolve it without
	// depending on payload schema (root-level vs nested meta).
	if event.TemplateID != "" {
		headers["templateId"] = event.TemplateID
	}

	// Use workspaceId as the partition key for co-located events.
	key := event.WorkspaceID
	if key == "" {
		key = event.EventID
	}

	return kafkautil.PublishEventTo(ctx, p.topic, key, event, headers)
}
