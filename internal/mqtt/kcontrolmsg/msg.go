// internal/mqtt/kcontrolmsg/msg.go
package kcontrolmsg

import (
	"context"
	"encoding/json"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
)

type HealthEvent struct {
	DeviceID        string      `json:"deviceId"`
	HWID            string      `json:"hwId"`
	HealthInterval  int64       `json:"healthInterval"`
	InactiveForMs   int64       `json:"inactiveForMs"`
	Kind            string      `json:"kind"`
	LastSeen        string      `json:"lastSeen"`
	Message         string      `json:"message"`
	Name            string      `json:"name"`
	OffThresholdMs  int64       `json:"offThresholdMs"`
	Sensor          interface{} `json:"sensor"`
	Source          string      `json:"source"`
	Stats           interface{} `json:"stats"`
	Status          string      `json:"status"`
	Timestamp       string      `json:"timestamp"`
	Topic           string      `json:"topic"`
	WarnThresholdMs int64       `json:"warnThresholdMs"`
}

type Bridge struct {
	mqtt mqtt.Client
	log  zerolog.Logger
}

func NewBridge(m mqtt.Client, baseLogger zerolog.Logger) *Bridge {
	// ให้ logger name เหมือนที่คุณโชว์: kctrlsvc.wiring.kcontrol-bridge
	l := baseLogger.With().
		Str("logger", "kctrlsvc.wiring.kcontrol-bridge").
		Logger()

	return &Bridge{mqtt: m, log: l}
}

// เรียกจาก Kafka consumer เวลาได้ message ใหม่
func (b *Bridge) HandleHealthEvent(ctx context.Context, raw []byte) error {
	var evt HealthEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		b.log.Warn().
			Err(err).
			Msg("invalid health event json")
		return err
	}

	if evt.Topic == "" {
		evt.Topic = "kcontrol.events"
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		b.log.Error().
			Err(err).
			Msg("marshal health event failed")
		return err
	}

	token := b.mqtt.Publish(evt.Topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		b.log.Error().
			Err(token.Error()).
			Str("topic", evt.Topic).
			Str("deviceId", evt.DeviceID).
			Msg("mqtt publish failed")
		return token.Error()
	}

	b.log.Info().
		Str("topic", evt.Topic).
		Str("deviceId", evt.DeviceID).
		Str("status", evt.Status).
		Msg("published health to mqtt")

	return nil
}

// ใช้ได้ถ้าคุณอยาก log ts เป็น float seconds แบบตัวอย่าง
func unixFloatSeconds() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
