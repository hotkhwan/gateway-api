// internal/mqtt/inframsg/publish.go
package inframsg

import (
	"encoding/json"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

var ErrNotConnected = errors.New("mqtt client not connected (inframsg)")

// PublishRaw ส่ง payload ดิบ ([]byte) ไป MQTT
func PublishRaw(topic string, qos byte, retain bool, payload []byte) error {
	log := logger.Boot("inframsg", "internal-mqtt-inframsg-PublishRaw")

	c := Client()
	if c == nil || !c.IsConnected() {
		log.Error().Str("topic", topic).Msg("❌ MQTT not connected (inframsg)")
		return ErrNotConnected
	}

	t := c.Publish(topic, qos, retain, payload)
	if t.Wait() && t.Error() != nil {
		log.Error().Err(t.Error()).Str("topic", topic).Msg("❌ MQTT publish failed (inframsg)")
		return t.Error()
	}

	log.Debug().Str("topic", topic).Msg("📤 MQTT message published (inframsg)")
	return nil
}

// PublishString ส่ง string payload
func PublishString(topic string, qos byte, retain bool, payload string) error {
	return PublishRaw(topic, qos, retain, []byte(payload))
}

// PublishJSON marshal v แล้วส่งเป็น JSON
func PublishJSON(topic string, qos byte, retain bool, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return PublishRaw(topic, qos, retain, b)
}
