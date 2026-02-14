// internal/kafka/kctrlcons/event.go
package kctrlcons

import (
	"context"
	"fmt"
	"os"
	"strings"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/services/kctrlsvc"
	"klynx/models/kctrlmod"
)

func StartKafkaEventConsumer(broker, topic string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "kctrlcons", "StartKctrlEventConsumer")
	log.Info().Msg("consumer_boot")

	log.Info().
		Str("broker", broker).
		Str("topic", topic).
		Msg("🟢 Starting Kafka Event Consumer for kcontrol")

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kctrl.events"
	}

	// group id: "kcontrol-events"
	kafka.StartConsumer(broker, topic, groupID, func(msg kctrlmod.EventMAPMessage) error {
		// โยนให้ service kctrlsvc ไปจัดการต่อ (รวมถึงส่ง MQTT)
		kctrlsvc.HandleEvent(ctx, msg)
		return nil
	})
}
