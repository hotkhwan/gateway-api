// internal/adapters/kafka/kctrlcons/health.go
package kctrlcons

import (
	"context"
	"fmt"
	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/services/kctrlsvc"
	"klynx/models/kctrlmod"
	"os"
	"strings"
)

func StartKafkaHealthConsumer(broker, topic string) {
	ctx := context.Background()
	// tracer := otel.Tracer("klynx/kctrlhcons")
	log := logger.FromCtx(ctx, "kctrlcons", "StartKctrlHealthConsumer")
	log.Info().Msg("consumer_boot")

	// groupID := "kwatch-watchlist"
	// log := logger.WithMeta("kctrlsvc", "StartKafkaHealthConsumer").With().Str("topic", topic).Logger()
	log.Info().Msg("🟢 Starting Kafka Health Consumer")

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kctrl.health"
	}

	kafka.StartConsumer(broker, topic, groupID, func(msg kctrlmod.HealthMessage) error {
		kctrlsvc.HandleHealth(ctx, msg)
		return nil
	})
}
