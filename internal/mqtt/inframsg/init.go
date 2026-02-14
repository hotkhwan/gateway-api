// internal/mqtt/inframsg/init.go
package inframsg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"klynx/internal/logger"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	client   mqtt.Client
	mqttOnce sync.Once
)

// Client คืน mqtt.Client กลาง (lazy init)
func Client() mqtt.Client {
	InitMQTT()
	return client
}

// InitMQTT initializes a shared MQTT client for infrastructure publishers.
func InitMQTT() {
	log := logger.Boot("inframsg", "internal-mqtt-inframsg-InitMQTT")

	mqttOnce.Do(func() {
		host := strings.TrimSpace(os.Getenv("MQTT_HOST"))
		if host == "" {
			host = "localhost"
		}

		port := strings.TrimSpace(os.Getenv("MQTT_PORT"))
		if port == "" {
			port = "8883"
		}

		serverName := host
		if s := strings.TrimSpace(os.Getenv("MQTT_SNI")); s != "" {
			// เผื่อกรณี SNI ต้องไม่ตรงกับ host (เช่น ผ่าน LB)
			serverName = s
		}

		broker := "tls://" + host + ":" + port
		clientID := fmt.Sprintf("klynx-api-infra-%d-%d", os.Getpid(), time.Now().UnixNano()%10000)

		caPath := os.Getenv("MQTT_CA_CERT")
		if strings.TrimSpace(caPath) == "" {
			log.Fatal().Msg("❌ MQTT_CA_CERT is empty for inframsg")
		}

		ca, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("path", caPath).
				Msg("❌ Failed to read CA file (inframsg)")
		}

		cp := x509.NewCertPool()
		if ok := cp.AppendCertsFromPEM(ca); !ok {
			log.Fatal().
				Str("path", caPath).
				Msg("❌ AppendCertsFromPEM failed (inframsg)")
		}

		tlsCfg := &tls.Config{
			RootCAs:    cp,
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		}

		opts := mqtt.NewClientOptions().
			AddBroker(broker).
			SetClientID(clientID).
			SetTLSConfig(tlsCfg).
			SetUsername(os.Getenv("MQTT_USERNAME")).
			SetPassword(os.Getenv("MQTT_PASSWORD")).
			SetKeepAlive(30 * time.Second).
			SetPingTimeout(10 * time.Second).
			SetAutoReconnect(true).
			SetConnectRetry(true).
			SetConnectRetryInterval(5 * time.Second).
			SetCleanSession(true)

		opts.OnConnect = func(c mqtt.Client) {
			log.Info().
				Str("broker", broker).
				Str("sni", serverName).
				Str("cafile", caPath).
				Msg("✅ Connected to MQTT broker (inframsg, publisher-only)")
			// publisher-only: ไม่ subscribe อะไรเลย
		}
		opts.OnConnectionLost = func(c mqtt.Client, err error) {
			log.Warn().Err(err).Msg("⚠️ MQTT connection lost (inframsg)")
		}
		opts.OnReconnecting = func(c mqtt.Client, _ *mqtt.ClientOptions) {
			log.Debug().Msg("🔄 MQTT reconnecting... (inframsg)")
		}

		client = mqtt.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Fatal().
				Err(token.Error()).
				Str("broker", broker).
				Str("sni", serverName).
				Str("cafile", caPath).
				Msg("❌ MQTT connection error (inframsg)")
		}
	})
}

func DisconnectMQTT() {
	log := logger.Boot("inframsg", "internal-mqtt-inframsg-DisconnectMQTT")
	if client != nil && client.IsConnected() {
		client.Disconnect(250)
		log.Info().Msg("✅ MQTT disconnected (inframsg)")
	}
}
