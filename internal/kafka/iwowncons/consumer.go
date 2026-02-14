package iwowncons

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"klynx/internal/kafka"
	"klynx/internal/logger"
)

type kindOnly struct {
	Kind string `json:"kind"`
}

func StartKafkaIwownConsumer(broker, topic string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "iwowncons", "StartKafkaIwownConsumer")

	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("KAFKA_TOPIC_IWOWN"))
		if topic == "" {
			topic = "kwatch4g.iwown"
		}
	}

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "iwown.consumer"
	}

	log.Info().
		Str("broker", broker).
		Str("topic", topic).
		Str("group", groupID).
		Msg("🟢 Starting Kafka IWOWN Consumer")

	kafka.StartConsumerWithHeaders(broker, topic, groupID,
		func(raw json.RawMessage, hdrs map[string]string) error {

			// 🔎 LOG ตอน consume (เห็นแน่นอนในระดับ INFO)
			var kind string
			_ = json.Unmarshal(raw, &struct {
				Kind string `json:"kind"`
			}{Kind: kind})

			// ดึง kind แบบปลอดภัย
			var k kindOnly
			_ = json.Unmarshal(raw, &k)

			log.Info().
				Int("len", len(raw)).
				Str("hdr_event", hdrs["event"]).
				Str("kind", k.Kind).
				Msg("📥 IWOWN Kafka message consumed")

			// 1) ใช้ header event ก่อน
			if ev := strings.TrimSpace(hdrs["event"]); ev != "" {
				log.Info().Str("event", ev).Msg("➡️ IWOWN route by header event")
				return HandleIwownEvent(ctx, "", raw, map[string]string{"event": ev})
			}

			// 2) fallback: อ่าน kind จาก body
			if strings.TrimSpace(k.Kind) == "" {
				log.Warn().Msg("⚠️ IWOWN no header event and no kind in body (drop)")
				return nil
			}

			// map kind -> event (inline เลย ไม่ต้อง mapKindToEvent)
			eventType := ""
			switch strings.ToLower(strings.TrimSpace(k.Kind)) {
			case "pb":
				eventType = "kwatch4g.pb"
			case "alarm":
				eventType = "kwatch4g.alarm"
			case "calllog":
				eventType = "kwatch4g.calllog"
			case "deviceinfo":
				eventType = "kwatch4g.deviceinfo"
			case "status":
				eventType = "kwatch4g.status"
			default:
				log.Warn().Str("kind", k.Kind).Msg("⚠️ IWOWN unknown kind (drop)")
				return nil
			}

			log.Info().Str("event", eventType).Msg("➡️ IWOWN route by kind")
			return HandleIwownEvent(ctx, "", raw, map[string]string{"event": eventType})
		},
	)
}
