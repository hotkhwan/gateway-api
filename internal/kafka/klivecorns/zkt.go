// internal/kafka/klivecorns/zkt.go
package klivecorns

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/klivesvc"
	"github.com/hotkhwan/gateway-api/models/klivemod"
)

// ใช้ carrier สำหรับ Extract W3C trace context จาก Kafka headers
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }
func (c mapCarrier) Set(key, val string)   { c[key] = val }
func (c mapCarrier) Keys() []string {
	out := make([]string, 0, len(c))
	for k := range c {
		out = append(out, k)
	}
	return out
}

func StartKliveConsumer(broker, topic string) {
	bg := context.Background()
	log := logger.FromCtx(bg, "klivecons", "StartKliveConsumer")

	// topic fallback
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("KAFKA_TOPIC_KLIVE"))
		if topic == "" {
			topic = "klive.player"
		}
	}

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "klive.consumer"
	}

	log.Debug().
		Str("broker", broker).
		Str("topic", topic).
		Str("group", groupID).
		Msg("🟢 Starting Kafka klive Consumer")

	// ✅ ใช้ตัวใหม่ที่มี headers
	kafka.StartConsumerWithHeaders(broker, topic, groupID, func(msg klivemod.LiveEvent, headers map[string]string) error {
		ev := strings.ToLower(strings.TrimSpace(msg.Event))
		if strings.TrimSpace(msg.ID) == "" {
			log.Warn().Str("event", ev).Msg("skip_empty_id")
			return nil
		}

		// ✅ Extract trace context จาก Kafka headers (traceparent/tracestate)
		parentCtx := otel.GetTextMapPropagator().Extract(context.Background(), mapCarrier(headers))

		// ✅ สร้าง span ของ consumer ต่อจาก producer
		tr := otel.Tracer("klynx.klivecons")
		ctx, span := tr.Start(parentCtx, "klive.consume")
		defer span.End()

		// debug สำคัญ
		log.Debug().
			Str("event", ev).
			Str("kliveID", msg.ID).
			Str("clientIp", strings.TrimSpace(msg.ClientIP)).
			Str("rawIp", strings.TrimSpace(msg.Raw.IP)).
			Str("traceparent", strings.TrimSpace(headers["traceparent"])).
			Msg("📥 klive kafka message received")

		return klivesvc.SaveKliveEvent(ctx, msg)
	})
}
