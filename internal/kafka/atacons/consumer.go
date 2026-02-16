// internal/kafka/atacons/event.go
package atacons

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/atasvc"
	"github.com/hotkhwan/gateway-api/models/aimodel"
)

// StartKafkaATAConsumer: consume ATA events จาก Kafka แล้วส่งต่อไป ATA service
// ใช้ signature มาตรฐานแบบเดียวกับตัวอื่น ๆ: (broker, topic string)
func StartKafkaATAConsumer(broker, topic string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "atacons", "StartKafkaATAConsumer")

	// ถ้า topic ที่ส่งเข้ามาว่าง ให้ fallback ไปใช้ env + default
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
		if topic == "" {
			topic = "ata.events"
		}
	}

	// groupID ใช้จาก env ถ้ามี, ถ้าไม่มีก็ fallback เป็น ata.consumer
	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "ata.consumer"
	}

	log.Debug().
		Str("broker", broker).
		Str("topic", topic).
		Str("group", groupID).
		Msg("🟢 Starting Kafka ATA Consumer")

	// consumer แบบเดียวกับตัวอื่น ๆ
	kafka.StartConsumer(broker, topic, groupID, func(msg aimodel.PusherEvent) error {
		log.Debug().
			Str("event", msg.Event).
			Int("data_len", len(msg.Data)).
			Msg("📥 ATA Kafka message received")

		// ส่งต่อให้ ATA service จัดการ MQTT + Persist
		return atasvc.HandleEvent(ctx, msg)
	})
}
