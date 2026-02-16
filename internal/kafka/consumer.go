// internal/kafka/consumer.go
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"

	"github.com/segmentio/kafka-go"
)

// -----------------------------
// Public APIs
// -----------------------------

// Legacy API (no headers)
func StartConsumer[T any](
	broker string,
	topic string,
	groupID string,
	handler func(T) error,
) {
	StartConsumerWithHeaders(
		broker,
		topic,
		groupID,
		func(msg T, _ map[string]string) error {
			return handler(msg)
		},
	)
}

// Header-aware consumer (W3C trace support)
func StartConsumerWithHeaders[T any](
	broker string,
	topic string,
	groupID string,
	handler func(T, map[string]string) error,
) {
	log := logger.WithMeta("kafka", topic+"-consumer").With().
		Str("topic", topic).
		Str("groupID", groupID).
		Logger()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1e3,              // 1KB
		MaxBytes:       10e6,             // 10MB
		MaxWait:        10 * time.Second, // ← ให้ Kafka long-poll เอง
		CommitInterval: 0,                // manual commit
	})

	defer func() {
		_ = reader.Close()
	}()

	log.Info().Msg("🟢 Kafka consumer started")

	for {
		// ❗ อย่าใส่ timeout สั้น ๆ ตรงนี้
		// Kafka Fetch เป็น long-poll อยู่แล้ว
		m, err := reader.FetchMessage(context.Background())
		if err != nil {

			// ✅ idle / rebalance / poll timeout = ปกติ
			if errors.Is(err, context.DeadlineExceeded) {
				log.Debug().Msg("kafka fetch idle timeout")
				continue
			}

			// ❌ error จริง
			log.Error().Err(err).Msg("❌ Kafka fetch error")
			time.Sleep(1 * time.Second)
			continue
		}

		// -----------------------------
		// Decode payload
		// -----------------------------
		var msg T
		if err := json.Unmarshal(m.Value, &msg); err != nil {
			log.Error().
				Err(err).
				Str("topic", topic).
				Int("partition", m.Partition).
				Int64("offset", m.Offset).
				Msg("❌ Failed to decode Kafka message")
			_ = reader.CommitMessages(context.Background(), m)
			continue
		}

		// -----------------------------
		// Extract headers
		// -----------------------------
		hdrs := make(map[string]string, len(m.Headers))
		for _, h := range m.Headers {
			hdrs[h.Key] = string(h.Value)
		}

		// -----------------------------
		// Call handler (business logic)
		// -----------------------------
		if err := handler(msg, hdrs); err != nil {
			log.Error().
				Err(err).
				Str("topic", topic).
				Int("partition", m.Partition).
				Int64("offset", m.Offset).
				Msg("❌ Kafka handler failed")

			// ⚠️ ตอนนี้เลือก "commit แล้วไปต่อ"
			// ถ้าต้องการ retry / DLQ → ทำเพิ่มตรงนี้
			_ = reader.CommitMessages(context.Background(), m)
			continue
		}

		// -----------------------------
		// Commit offset (success path)
		// -----------------------------
		if err := reader.CommitMessages(context.Background(), m); err != nil {
			log.Error().
				Err(err).
				Str("topic", topic).
				Int("partition", m.Partition).
				Int64("offset", m.Offset).
				Msg("❌ Commit offset failed")
		}
	}
}
