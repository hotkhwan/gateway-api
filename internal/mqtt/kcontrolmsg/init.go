// internal/adapters/mqtt/kcontrol/init.go
package kcontrolmsg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"klynx/internal/logger"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	MqttClient mqtt.Client
	mqttOnce   sync.Once
)

func InitMQTT() {
	log := logger.Boot("kcontrol", "internal-mqtt-InitMQTT")
	mqttOnce.Do(func() {
		host := os.Getenv("MQTT_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("MQTT_PORT")
		if port == "" {
			port = "8883"
		}
		serverName := strings.TrimSpace(os.Getenv("MQTT_HOST"))
		if serverName == "" {
			serverName = host
		}

		broker := "tls://" + host + ":" + port
		clientID := fmt.Sprintf("klynx-api-%d-%d", os.Getpid(), time.Now().UnixNano()%10000)

		caPath := os.Getenv("MQTT_CA_CERT") // ต้องเป็น “Root CA” ที่ใช้เซ็น ไม่ใช่ mosquitto.crt
		ca, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatal().Err(err).Str("path", caPath).Msg("❌ Failed to read CA file")
		}
		cp := x509.NewCertPool()
		if ok := cp.AppendCertsFromPEM(ca); !ok {
			log.Fatal().Str("path", caPath).Msg("❌ AppendCertsFromPEM failed")
		}

		// พยายามไม่ใช้ InsecureSkipVerify
		tlsCfg := &tls.Config{
			RootCAs:    cp,
			ServerName: serverName, // ต้องตรง SAN
			MinVersion: tls.VersionTLS12,
		}

		// ส่ง log ของ paho ออก stdout (ง่ายและ compile ผ่านแน่)
		// mqtt.ERROR = os.Stdout
		// mqtt.CRITICAL = os.Stdout
		// mqtt.WARN = os.Stdout
		// mqtt.DEBUG = os.Stdout

		opts := mqtt.NewClientOptions().
			AddBroker(broker).
			SetClientID(clientID).
			SetTLSConfig(tlsCfg).
			SetUsername(os.Getenv("MQTT_USERNAME")).
			SetPassword(os.Getenv("MQTT_PASSWORD")).
			SetCleanSession(true).
			SetConnectTimeout(10 * time.Second).
			SetKeepAlive(30 * time.Second).
			SetPingTimeout(10 * time.Second).
			SetAutoReconnect(true).
			SetConnectRetry(true).
			SetConnectRetryInterval(5 * time.Second)

		opts.OnConnect = func(c mqtt.Client) {
			log.Info().
				Str("broker", broker).
				Str("sni", serverName).
				Str("cafile", caPath).
				Msg("✅ Connected to MQTT broker")

			topics := map[string]byte{
				"kcontrol.alarms":   2,
				"kcontrol.health":   2,
				"kcontrol.response": 2,
				"kcontrol.sensor":   2,
			}
			if token := c.SubscribeMultiple(topics, MessageHandler); token.Wait() && token.Error() != nil {
				log.Error().Err(token.Error()).Msg("❌ Failed to subscribe to MQTT topics")
			}
		}
		opts.OnConnectionLost = func(c mqtt.Client, err error) {
			log.Warn().Err(err).Msg("⚠️ MQTT connection lost")
		}
		opts.OnReconnecting = func(c mqtt.Client, _ *mqtt.ClientOptions) {
			log.Debug().Msg("🔄 MQTT reconnecting...")
		}

		MqttClient = mqtt.NewClient(opts)
		tok := MqttClient.Connect()
		if tok.Wait() && tok.Error() != nil {
			log.Fatal().
				Err(tok.Error()).
				Str("broker", broker).
				Str("sni", serverName).
				Str("cafile", caPath).
				Msg("❌ MQTT connection error")
		}
	})
}

func DisconnectMQTT() {
	log := logger.Boot("kcontrol", "internal-mqtt-DisconnectMQTT")
	if MqttClient != nil && MqttClient.IsConnected() {
		MqttClient.Disconnect(250)
		log.Info().Msg("✅ MQTT disconnected")
	}
}

func PublishToMQTT(topic, payload string) error {
	log := logger.Boot("kcontrol", "internal-mqtt-PublishToMQTT")
	if token := MqttClient.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		log.Error().Err(token.Error()).Str("topic", topic).Msg("❌ MQTT publish failed")
		return fmt.Errorf("MQTT publish error: %w", token.Error())
	}
	log.Debug().Str("topic", topic).Msg("📤 MQTT message published")
	return nil
}
