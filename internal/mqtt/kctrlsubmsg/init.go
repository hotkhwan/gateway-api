// internal/mqtt/kctrlsubmsg/init.go
package kctrlsubmsg

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// kctrlsubmsg is the gateway-api MQTT subscriber that consolidates kcontrol
// device traffic from the shared MQTT broker and republishes it onto the
// canonical gw.kcontrol.*.v1 Kafka topics. It is the producer side of the
// klynx-realtime-restoration Phase 0 plan
// (klynx-api/docs/plan/realtime-restoration.md §9 Phase 0).
//
// This package is intentionally a thin forwarder: it parses the incoming
// MQTT payload, builds a canonical envelope keyed by hwId, and writes to
// Kafka. Domain projection (Mongo upsert, approved-device filter) lives at
// the downstream klynx-api consumers (Phase 2, gwkctrl*cons) so that
// ownership stays "gateway-api owns the MQTT->Kafka boundary, klynx-api
// owns its own projection store".

var (
	client   mqtt.Client
	mqttOnce sync.Once
)

// InitMQTT initialises the TLS-secured MQTT subscriber for the kcontrol
// topic family. Idempotent — repeated calls are no-ops after the first.
//
// Required env:
//
//	MQTT_HOST       — broker hostname (defaults to localhost)
//	MQTT_PORT       — broker port    (defaults to 8883)
//	MQTT_CA_CERT    — path to Root CA used by the broker certificate
//	MQTT_USERNAME   — broker username
//	MQTT_PASSWORD   — broker password
//
// Optional env:
//
//	MQTT_SNI        — TLS SNI hostname (defaults to MQTT_HOST)
func InitMQTT() {
	log := logger.Boot("kctrlsubmsg", "internal-mqtt-kctrlsubmsg-InitMQTT")

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
			serverName = s
		}

		broker := "tls://" + host + ":" + port
		clientID := fmt.Sprintf("gateway-api-kctrlsub-%d-%d", os.Getpid(), time.Now().UnixNano()%10000)

		caPath := os.Getenv("MQTT_CA_CERT")
		if strings.TrimSpace(caPath) == "" {
			log.Fatal().Msg("❌ MQTT_CA_CERT is empty for kctrlsubmsg")
		}

		ca, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("path", caPath).
				Msg("❌ Failed to read CA file (kctrlsubmsg)")
		}

		cp := x509.NewCertPool()
		if ok := cp.AppendCertsFromPEM(ca); !ok {
			log.Fatal().
				Str("path", caPath).
				Msg("❌ AppendCertsFromPEM failed (kctrlsubmsg)")
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
				Str("clientId", clientID).
				Msg("✅ Connected to MQTT broker (kctrlsubmsg)")

			// QoS 2 — exactly-once delivery for device events. Same QoS used by
			// the legacy klynx-api kcontrolmsg subscriber so behaviour is
			// indistinguishable from the broker's point of view during the
			// dual-publish window.
			topics := map[string]byte{
				"kcontrol.health":   2,
				"kcontrol.alarms":   2,
				"kcontrol.sensor":   2,
				"kcontrol.events":   2,
				"kcontrol.response": 2,
			}
			if token := c.SubscribeMultiple(topics, MessageHandler); token.Wait() && token.Error() != nil {
				log.Error().Err(token.Error()).Msg("❌ Failed to subscribe to MQTT topics (kctrlsubmsg)")
				return
			}
			log.Info().
				Int("topicCount", len(topics)).
				Msg("kctrlsubmsg ready: subscribed to K-Control MQTT topics")
		}
		opts.OnConnectionLost = func(c mqtt.Client, err error) {
			log.Warn().Err(err).Msg("⚠️ MQTT connection lost (kctrlsubmsg)")
		}
		opts.OnReconnecting = func(c mqtt.Client, _ *mqtt.ClientOptions) {
			log.Debug().Msg("🔄 MQTT reconnecting... (kctrlsubmsg)")
		}

		client = mqtt.NewClient(opts)

		// Connect is non-blocking on boot: paho's ConnectRetry=true keeps
		// trying in the background until the first successful connection,
		// at which point OnConnect fires and subscribes. Bounded WaitTimeout
		// lets boot record a definitive log line ("connected" or "still
		// trying") without blocking the rest of main() — previously a hard
		// Wait() would freeze startup forever if the broker was unreachable
		// or the first TLS handshake stalled.
		token := client.Connect()
		if !token.WaitTimeout(10 * time.Second) {
			log.Warn().
				Str("broker", broker).
				Str("sni", serverName).
				Str("clientId", clientID).
				Msg("⚠️ MQTT connect still in progress after 10s — continuing boot; paho will keep retrying in background (kctrlsubmsg)")
			return
		}
		if err := token.Error(); err != nil {
			log.Error().
				Err(err).
				Str("broker", broker).
				Str("sni", serverName).
				Str("clientId", clientID).
				Msg("❌ MQTT connect error — paho will keep retrying in background (kctrlsubmsg)")
		}
	})
}

// DisconnectMQTT cleanly disconnects the subscriber. Called from the main
// shutdown hook so the broker logs a clean DISCONNECT and not a timeout.
func DisconnectMQTT() {
	log := logger.Boot("kctrlsubmsg", "internal-mqtt-kctrlsubmsg-DisconnectMQTT")
	if client != nil && client.IsConnected() {
		client.Disconnect(250)
		log.Info().Msg("✅ MQTT disconnected (kctrlsubmsg)")
	}
}
