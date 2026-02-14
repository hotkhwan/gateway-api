// internal/adapters/kafka/kctrlcons/alarm.go
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

func StartKafkaAlarmConsumer(broker, topic string) {
	ctx := context.Background()
	// tracer := otel.Tracer("klynx/Kctrlcons")
	log := logger.FromCtx(ctx, "kctrlcons", "StartKctrlAlarmConsumer")
	log.Debug().Msg("consumer_boot")

	// groupID := "kctrl-alarms"
	// log := logger.WithMeta("kctrlsvc", "StartKafkaHealthConsumer").With().Str("topic", topic).Logger()
	log.Debug().Msg("🟢 Starting Kafka Alarm Consumer")

	// groupID fallback
	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kctrl.alarms"
	}

	kafka.StartConsumer(broker, topic, groupID, func(msg kctrlmod.AlarmMessage) error {
		kctrlsvc.HandleAlarm(ctx, msg)
		return nil
	})
}
