// internal/mqtt/kwatchmsg/init.go
package kwatchmsg

import (
	"klynx/internal/logger"
	"klynx/internal/mqtt/inframsg"
)

// InitMQTT: backward-compatible wrapper → ใช้ MQTT client กลางจาก inframsg
func InitMQTT() {
	log := logger.Boot("kwatchmsg", "internal-mqtt-kwatchmsg-InitMQTT")
	log.Debug().Msg("🔧 InitMQTT (kwatchmsg) delegate to inframsg.InitMQTT")
	inframsg.InitMQTT()
}

// DisconnectMQTT: wrapper ปิด client กลาง (ถ้าอยากแยก lifecycle ก็ไปจัดใน inframsg แทน)
func DisconnectMQTT() {
	log := logger.Boot("kwatchmsg", "internal-mqtt-kwatchmsg-DisconnectMQTT")
	log.Debug().Msg("🔧 DisconnectMQTT (kwatchmsg) delegate to inframsg.DisconnectMQTT")
	inframsg.DisconnectMQTT()
}
