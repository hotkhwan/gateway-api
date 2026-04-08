// internal/kafka/klynxdeliverycons/consumer.go
package klynxdeliverycons

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/gateways/klynxgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// StartKlynxDeliveryConsumer consumes phibek.delivery.events.v1 and forwards each event
// to klynx-api via HTTPS POST (saasPublic profile only).
//
// Must only be started when DEPLOYMENT_PROFILE=saasPublic.
// For appliance profile, phibek hands off via EventBridge (Kafka internal).
func StartKlynxDeliveryConsumer(ctx context.Context) {
	log := logger.Boot("klynxdeliverycons", "StartKlynxDeliveryConsumer")

	client, err := klynxgw.New()
	if err != nil {
		log.Error().Err(err).Msg("klynxgw client init failed — klynx delivery consumer not started")
		return
	}

	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_PHIBEK_DELIVERY", "phibek.delivery.events.v1")
	groupID := "phibek-klynx-delivery-grp"

	consLog := log.With().Str("topic", topic).Str("group", groupID).Logger()

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

	consLog.Info().Msg("klynx delivery consumer started")

	for {
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				consLog.Info().Msg("klynx delivery consumer shutting down")
				return
			}
			consLog.Error().Err(err).Msg("[klynxdelivery] fetch error")
			time.Sleep(time.Second)
			continue
		}

		handleDelivery(m, client, consLog)
		_ = reader.CommitMessages(ctx, m)
	}
}

func handleDelivery(m kafka.Message, client *klynxgw.Client, log zerolog.Logger) {
	// restore trace context from Kafka producer headers
	hdrs := make(map[string]string, len(m.Headers))
	for _, h := range m.Headers {
		hdrs[h.Key] = string(h.Value)
	}
	parentCtx := traceutil.ExtractHeaders(context.Background(), hdrs)
	ctx, end, spanLog := traceutil.StartLite(
		parentCtx,
		"github.com/hotkhwan/gateway-api/klynxdelivery",
		"klynxdelivery.handle",
		"klynxdeliverycons", "handle",
	)
	defer end()

	var event eventschema.NormalizedEvent
	if err := json.Unmarshal(m.Value, &event); err != nil {
		log.Error().Err(err).
			Int("partition", m.Partition).Int64("offset", m.Offset).
			Msg("[klynxdelivery] decode failed — skipping")
		return
	}

	if err := client.Send(ctx, event); err != nil {
		spanLog.Error().Err(err).Str("eventId", event.EventID).Msg("[klynxdelivery] send to klynx failed")
		return
	}

	spanLog.Debug().Str("eventId", event.EventID).Msg("[klynxdelivery] event delivered to klynx-api")
}
