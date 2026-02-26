// config/kafka.go
package config

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"

	"github.com/segmentio/kafka-go"
)

var (
	KafkaWriter *kafka.Writer
	kafkaOnce   sync.Once
)

func InitKafka() { _ = ensureKafkaWriter() }

func ensureKafkaWriter() error {
	var initErr error
	kafkaOnce.Do(func() {
		log := logger.Boot("kafka", "config-InitKafka")
		broker := os.Getenv("KAFKA_BROKER")
		if broker == "" {
			initErr = errors.New("KAFKA_BROKER not set")
			log.Error().Msg("❌ KAFKA_BROKER not set")
			return
		}

		topics := splitCSV(os.Getenv("KAFKA_ENSURE_TOPICS"))
		topics = append(topics, topicsWithDefaults([][2]string{
			{"KAFKA_TOPIC", "result"},
			{"KAFKA_AI_TOPIC", "ksearch.cam"},
			{"KAFKA_DETECTION_TOPIC", "tp.detection"},
			{"KAFKA_KALERT_TOPIC", "kalert"},
			{"KAFKA_KCONTROL_ALARMS_TOPIC", "kcontrol.alarms"},
			{"KAFKA_KCONTROL_EVENTS_TOPIC", "kcontrol.events"},
			{"KAFKA_KCONTROL_HEALTH_TOPIC", "kcontrol.health"},
			{"KAFKA_KDETECT_TOPIC", "kdetect"},
			{"KAFKA_KSEARCH_CAM_TOPIC", "ksearch.cam"},
			{"KAFKA_KSEARCH_VIDEO_TOPIC", "ksearch.video"},
			{"KAFKA_KWATCH_WATCHLIST_TOPIC", "kwatch.watchlist"},
			{"KAFKA_KWATCH_WATCHLIST_SYNC_TOPIC", "kwatch.watchlist.sync"},
			{"KAFKA_LIVE_TOPIC", "klive.player"},
			{"KAFKA_TOPIC_IBOC", "iboc"},
			{"KAFKA_TOPIC_ATA", "ata.events"},
			{"KAFKA_TOPIC_RAW_EVENTS", "raw.events"},
		})...)

		// (ถ้าต้องการ topic เพิ่มเติมเฉพาะกิจ ก็ append ต่อได้)
		// topics = append(topics, "some.extra.topic")

		// เพิ่ม topic งาน sync ถ้าใช้
		topics = append(topics, "kwatch.watchlist.sync")

		// ② ensure ทุก topic
		parts := envInt("KAFKA_TOPIC_PARTITIONS", 6)
		rf := int16(envInt("KAFKA_TOPIC_RF", 1)) // single-broker → 1
		for _, t := range dedupe(topics) {
			if t == "" {
				continue
			}
			if err := ensureTopic(broker, t, parts, rf, map[string]string{
				"cleanup.policy":   "delete",
				"compression.type": "producer",
				// "retention.ms":   os.Getenv("KAFKA_TOPIC_RETENTION_MS"),
			}); err != nil {
				log.Error().Err(err).Str("topic", t).Msg("❌ ensure topic failed")
			} else {
				log.Info().Str("topic", t).Msg("✅ topic ensured")
			}
		}

		KafkaWriter = &kafka.Writer{
			Addr:                   kafka.TCP(broker),
			Balancer:               &kafka.LeastBytes{},
			RequiredAcks:           kafka.RequireAll,
			WriteTimeout:           10 * time.Second,
			Async:                  false,
			AllowAutoTopicCreation: true, // เผื่อ broker เปิดไว้
			BatchSize:              envInt("KAFKA_BATCH_SIZE", 1000),
			BatchTimeout:           envDur("KAFKA_BATCH_TIMEOUT", 50*time.Millisecond),
			BatchBytes:             envInt64("KAFKA_BATCH_BYTES", 1<<20),
			Compression:            kafka.Snappy, // ← เปลี่ยนจาก CompressionCodec
		}
		log.Info().Str("broker", broker).Msg("✅ Kafka writer initialized")
	})
	return initErr
}

func SendToKafkaWithCtx(ctx context.Context, topic, key string, val []byte, headers map[string]string) error {
	log := logger.Boot("kafka", "config-SendToKafkaWithCtx")
	if KafkaWriter == nil {
		if err := ensureKafkaWriter(); err != nil {
			log.Error().Err(err).Str("topic", topic).Msg("❌ Kafka not initialized")
			return err
		}
	}
	var hs []kafka.Header
	for k, v := range headers {
		hs = append(hs, kafka.Header{Key: k, Value: []byte(v)})
	}

	log.Debug().
		Str("topic", topic).
		Str("key", key).
		Int("bytes", len(val)).
		Msg("📤 writing to kafka")

	return KafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   val,
		Headers: hs,
	})
}

// -------------------- helpers เพิ่มเติม --------------------
func TopicEnv(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v != "" {
		return v
	}
	return def
}

func topicsWithDefaults(pairs [][2]string) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, TopicEnv(p[0], p[1]))
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if x := strings.TrimSpace(p); x != "" {
			out = append(out, x)
		}
	}
	return out
}
func dedupe(in []string) []string {
	m := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := m[v]; ok {
			continue
		}
		m[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// สำหรับโค้ดเก่าที่ไม่มี ctx
func SendToKafkaWith(topic, key string, val []byte, headers map[string]string) error {
	return SendToKafkaWithCtx(context.Background(), topic, key, val, headers)
}

// เรียกตอน shutdown
func CloseKafka() {
	if KafkaWriter != nil {
		_ = KafkaWriter.Close()
	}
}

// -------------------- helpers เพิ่มเติม --------------------

func envInt(name string, def int) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// helper เพิ่ม
func envInt64(name string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envDur(name string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// ensureTopic: สร้าง topic ถ้ายังไม่มี (เช็คก่อน → ค่อยยิง CreateTopics ไปที่ controller)
func ensureTopic(broker, topic string, partitions int, rf int16, cfg map[string]string) error {
	// 1) dial broker ใดๆ เพื่ออ่าน metadata
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()

	// ถ้ามีอยู่แล้วก็จบ
	if parts, err := conn.ReadPartitions(topic); err == nil && len(parts) > 0 {
		return nil
	}

	// 2) หา controller แล้วคุยกับ controller เพื่อสร้าง topic
	ctrl, err := conn.Controller()
	if err != nil {
		return err
	}
	ctrlAddr := net.JoinHostPort(ctrl.Host, strconv.Itoa(ctrl.Port))
	cc, err := kafka.Dial("tcp", ctrlAddr)
	if err != nil {
		return err
	}
	defer cc.Close()

	// แปลง config map → แบบที่ kafka-go รองรับ
	entries := make([]kafka.ConfigEntry, 0, len(cfg))
	for k, v := range cfg {
		vv := v
		entries = append(entries, kafka.ConfigEntry{
			ConfigName:  k,
			ConfigValue: vv, // ← เดิมเป็น &vv
		})
	}

	return cc.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: int(rf),
		ConfigEntries:     entries, // ← เดิมเป็น map[string]*string
	})
}
