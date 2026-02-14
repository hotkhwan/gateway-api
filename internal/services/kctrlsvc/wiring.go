// internal/services/kctrlsvc/wiring.go
package kctrlsvc

import (
	"sync"

	"github.com/rs/zerolog"

	"klynx/internal/mqtt/kcontrolmsg"
)

type gateways struct {
	MQTTBridge *kcontrolmsg.Bridge
}

var (
	gwOnce sync.Once
	gw     gateways
)

// สมมติคุณมี root logger จากระบบอยู่แล้ว
// ถ้ายังไม่มี ให้ส่งเข้ามาจากที่ init service ดีกว่า (อย่า New logger ซ้ำมั่วๆ ใน package)
var baseLogger zerolog.Logger

func SetLogger(l zerolog.Logger) {
	baseLogger = l
}

func getGateways() gateways {
	gwOnce.Do(func() {
		gw = initGatewaysFromEnv()
	})
	return gw
}

func initGatewaysFromEnv() gateways {
	log := baseLogger.With().
		Str("logger", "kctrlsvc.wiring").
		Logger()

	// ✅ ใช้ MQTT client ตัวเดียวกับที่ init.go สร้างไว้
	kcontrolmsg.InitMQTT()

	client := kcontrolmsg.MqttClient
	if client == nil || !client.IsConnected() {
		log.Error().
			Msg("MQTT client from InitMQTT is nil or not connected; MQTT bridge disabled")
		return gateways{}
	}

	log.Info().
		Msg("MQTT bridge configured via existing InitMQTT")

	bridge := kcontrolmsg.NewBridge(client, baseLogger) // NewBridge จะไปใส่ logger name ให้เอง
	return gateways{MQTTBridge: bridge}
}
