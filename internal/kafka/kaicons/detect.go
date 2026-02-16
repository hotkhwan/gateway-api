// internal/adapters/services/kaisvc/consumer.go
package kaicons

import (
	"context"
	"fmt"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/kaisvc"
	"github.com/hotkhwan/gateway-api/models/kaimod"
	"os"
	"strings"
)

func StartKafkaDetectConsumer(broker, topic string) {
	ctx := context.Background()
	// tracer := otel.Tracer("github.com/hotkhwan/gateway-api/kwatchcons")
	log := logger.FromCtx(ctx, "kwatchcons", "StartWatchlistConsumer")
	log.Info().Msg("🧠 Starting Kafka Detection Consumer")

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kai.detect"
	}

	kafka.StartConsumer(broker, topic, groupID, func(msg kaimod.Detection) error {
		log.Debug().Str("NameMethod", msg.Details.Analyze.Detection.NameMethod).
			Msg("📥 Detection message received")
		go kaisvc.HandleDetect(ctx, msg)
		return nil
	})
}
