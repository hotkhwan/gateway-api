// internal/adapters/kafka/kctrlcons/sensor.go
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

func StartKafkaSensorConsumer(broker, topic string) {
	ctx := context.Background()
	// tracer := otel.Tracer("klynx/Kctrlcons")
	log := logger.FromCtx(ctx, "Kctrlcons", "StartKctrlSensorConsumer")
	log.Info().Msg("consumer_boot")

	log.Info().Msg("🟢 Starting Kafka Sensor Consumer")
	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kctrl.sensors"
	}

	kafka.StartConsumer(broker, topic, groupID, func(msg kctrlmod.SensorMessage) error {
		kctrlsvc.HandleSensor(ctx, msg)
		return nil
	})
}
