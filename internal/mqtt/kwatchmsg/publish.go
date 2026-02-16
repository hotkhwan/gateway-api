// internal/mqtt/kwatchmsg/publish.go
package kwatchmsg

import (
	"encoding/json"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/mqtt/inframsg"
)

func TopicMsg() string           { return "ui/msg" }
func TopicStream() string        { return "ui/watchlist/stream" }
func TopicByID(id string) string { return "ui/watchlist/" + id }

func PublishToMsg(event string, payload any) error {
	return PublishJSON(TopicMsg(), 0, false, map[string]any{
		"event":   event,
		"payload": payload,
		"time":    time.Now().UTC(),
	})
}

func PubTopicToMsg(topic, event string, payload any) error {
	return PublishJSON(topic, 0, false, map[string]any{
		"message": event,
		"payload": payload,
		"time":    time.Now().UTC(),
	})
}

func PublicEvent(event string) error {
	lg := logger.Boot("kwatchmsg", "PublishPublicEvent")
	topic := TopicPublicEvents()
	payload := map[string]string{"event": event}

	lg.Info().Str("topic", topic).Str("event", event).Msg("📤 MQTT public: try publish")

	if err := PublishJSON(topic, 0, false, payload); err != nil {
		lg.Error().
			Err(err).
			Str("topic", topic).
			Str("event", event).
			Msg("❌ MQTT public publish failed")
		return err
	}

	lg.Info().Str("topic", topic).Str("event", event).Msg("✅ MQTT public published")
	return nil
}

func TopicPublicEvents() string {
	if t := os.Getenv("MQTT_PUBLIC_TOPIC"); t != "" {
		return t
	}
	return "public/watchlist/events"
}

// ---------- delegate ไป inframsg ----------

func PublishRaw(topic string, qos byte, retain bool, payload []byte) error {
	// ใช้ infra กลาง ไม่ใช้ Client ของตัวเองแล้ว
	return inframsg.PublishRaw(topic, qos, retain, payload)
}

func PublishJSON(topic string, qos byte, retain bool, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return PublishRaw(topic, qos, retain, b)
}
