// internal/adapters/services/authzsvc/consumer.go
// StartKafkaAuthzRelationshipConsumer → REST Version
package authzcons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/models/authzmod"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func StartKafkaAuthzRelationshipConsumer(broker, topic string) {
	base := context.Background()
	tracer := otel.Tracer("klynx/authzcons")
	boot := logger.FromCtx(base, "authzcons", "StartKafkaAuthzRelationshipConsumer")
	boot.Info().Str("topic", topic).Msg("consumer_boot")
	boot.Info().Msg("🟢 Starting Kafka AuthZ Relationship Consumer")

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "authz.relationship"
	}

	kafka.StartConsumerWithHeaders(broker, topic, groupID,
		func(m authzmod.RelationshipEvent, headers map[string]string) error {
			// งบเวลาต่อข้อความ
			ctx, cancel := context.WithTimeout(base, 30*time.Second)
			defer cancel()

			// Extract W3C trace context จาก Kafka headers
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))

			// สร้าง consumer span
			var span trace.Span
			ctx, span = tracer.Start(ctx, "Authz.RelationshipEvent")
			span.SetAttributes(
				attribute.String("kafka.topic", topic),
				// เพิ่ม attributes อื่น ๆ ได้ถ้า struct มีฟิลด์รองรับ
			)
			defer span.End()

			log := logger.FromCtx(ctx, "authzcons", "RelationshipConsumer").With().
				Str("topic", topic).
				Logger()

			log.Info().Msg("consumer_received")

			// ✅ เรียก handler แบบรับ ctx
			// if err := authzsvc.HandleRelationshipEvent(ctx, m); err != nil {
			// 	log.Error().Err(err).Msg("authz handler failed")
			// 	return err
			// }

			log.Info().Msg("authz handler done")
			return nil
		},
	)
}
