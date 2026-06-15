// internal/kafka/kctrlpubcons/consumer.go
//
// Phase 4b.1 consumer of klynx.kcontrol.commands.v1 per
// docs/contracts/kcontrol-outbound-commands.md (Rev 3, in
// klynx-api/docs/contracts/). Decodes the envelope, validates against
// the topic allowlist (defense-in-depth — klynx-api producer also
// validates), gates on the shadow-mode env, and publishes to MQTT.
//
// Shadow-mode default: GW_KCONTROL_PUBLISH_ENABLED=false → log envelope
// metadata only, do NOT publish. Operator flips at contract §9.2.1
// step 4 to begin live mode.
//
// Envelope logs are redacted per contract §8.1: topic / hwId / traceId /
// qos / retain / producedAt / payloadBytes / payloadSha256 ONLY. Raw
// payload MUST NEVER appear at any log level (Rev 3 — no escape hatch).
package kctrlpubcons

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/mqtt/inframsg"
)

// CommandEnvelope mirrors the klynx-api producer wire shape per
// contract §5.1. JSON tags MUST stay in sync — this is the
// bilateral contract.
type CommandEnvelope struct {
	Topic      string `json:"topic"`
	Payload    string `json:"payload"`
	QoS        int    `json:"qos,omitempty"`
	Retain     bool   `json:"retain,omitempty"`
	HwID       string `json:"hwId,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	ProducedAt string `json:"producedAt"`
}

// allowedTopics — contract §5.1.1 closed set. MUST match the klynx-api
// producer allowlist exactly (defense-in-depth). Both sides reject any
// other topic; consumer-side rejection drops the envelope without
// touching the MQTT broker.
var allowedTopics = map[string]struct{}{
	"kcontrol.control":     {},
	"kcontrol.alarmResult": {},
	"kcontrol/alarms":      {},
}

// payloadMaxBytes is the per-envelope payload cap per contract §5.3.
const payloadMaxBytes = 64 * 1024

// Indirections so tests can stub the MQTT publisher and the env reader
// without spinning up a broker.
var (
	publishRawFn = inframsg.PublishRaw

	// publishEnabled reads the GW_KCONTROL_PUBLISH_ENABLED env each call —
	// operator hot-reload (no pod restart) flips shadow → live at
	// contract §9.2.1 step 4. Unknown values default to FALSE (safe —
	// shadow mode does not publish, so an env typo cannot accidentally
	// double-send commands during cutover).
	publishEnabled = func() bool {
		v := strings.TrimSpace(os.Getenv("GW_KCONTROL_PUBLISH_ENABLED"))
		return strings.EqualFold(v, "true")
	}
)

// envCommandsTopic is the Kafka topic name. Defaults to the contract
// canonical value; operator override via env.
const envCommandsTopic = "KAFKA_TOPIC_KLYNX_KCONTROL_COMMANDS"
const defaultCommandsTopic = "klynx.kcontrol.commands.v1"

// CommandsTopic returns the configured topic, default matches contract §4.
func CommandsTopic() string {
	if v := strings.TrimSpace(os.Getenv(envCommandsTopic)); v != "" {
		return v
	}
	return defaultCommandsTopic
}

// StartConsumer wires the Kafka reader and the per-message handler.
// Blocking; intended to run in its own goroutine from main.go (mirrors
// how kctrlsubmsg + the other kctrl consumers are wired in this repo).
func StartConsumer(broker string) {
	ctx := context.Background()
	log := logger.FromCtx(ctx, "kctrlpubcons", "StartConsumer")
	topic := CommandsTopic()

	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	groupID := "gw-api.kctrlpubcons.v1"
	if baseGroup != "" {
		// Match the pattern used by sibling consumers in this repo —
		// optional namespacing for multi-tenant Kafka deployments.
		groupID = fmt.Sprintf("%s.%s", baseGroup, "kctrlpubcons.v1")
	}

	log.Info().
		Str("topic", topic).
		Str("groupID", groupID).
		Bool("publishEnabled", publishEnabled()).
		Msg("🟢 Starting kctrl outbound bridge consumer")

	kafka.StartConsumerWithHeaders(broker, topic, groupID, func(env CommandEnvelope, _ map[string]string) error {
		return handleEnvelope(ctx, env)
	})
}

// handleEnvelope is the per-message processing loop per contract §7.2.
// Exported via the StartConsumer wrapper above; broken out so tests can
// drive the handler directly without spinning up Kafka.
func handleEnvelope(ctx context.Context, env CommandEnvelope) error {
	log := logger.FromCtx(ctx, "kctrlpubcons", "handleEnvelope")
	redacted := redactedFields(env)

	// 2. Topic allowlist — contract §5.1.1.
	if _, ok := allowedTopics[env.Topic]; !ok {
		log.Warn().
			Fields(redacted).
			Msg("kctrl outbound topic rejected (not on allowlist)")
		// Counter: kctrlpubcons.topic.rejected. Metric system wiring is
		// a follow-up — at minimum the log line above is greppable for
		// alerting until the metrics pipeline is plugged in.
		return nil // commit + skip; bad envelope is unrecoverable
	}

	// 3. Payload size cap — contract §5.3.
	if len(env.Payload) > payloadMaxBytes {
		log.Warn().
			Fields(redacted).
			Int("payloadBytesLimit", payloadMaxBytes).
			Msg("kctrl outbound payload oversize — dropped")
		return nil
	}

	// 4. Shadow-mode check — contract §9.2 step gating.
	if !publishEnabled() {
		log.Info().
			Fields(redacted).
			Msg("kctrl outbound shadow-mode (publish skipped)")
		return nil
	}

	// 5. Live mode — publish to MQTT.
	if err := publishRawFn(env.Topic, byte(env.QoS), env.Retain, []byte(env.Payload)); err != nil {
		log.Error().
			Err(err).
			Fields(redacted).
			Msg("kctrl outbound MQTT publish failed")
		// Fire-and-forget semantics match today's klynx-api direct
		// publish — commit offset rather than block the partition.
		return nil
	}

	log.Info().
		Fields(redacted).
		Msg("kctrl outbound MQTT publish ok")
	return nil
}

// redactedFields returns the log shape per contract §8.1. NEVER
// includes `env.Payload`. Caller passes the map to `log.Fields(...)`.
//
// Codex round-3 blocker #2: there is no escape hatch. Even debug paths
// MUST use this redacted shape. If a specific malformed envelope needs
// investigation, the operator obtains a sanitized repro payload from
// the producer side — production logs do not carry raw bytes.
func redactedFields(env CommandEnvelope) map[string]interface{} {
	sum := sha256.Sum256([]byte(env.Payload))
	sha16 := hex.EncodeToString(sum[:])[:16]
	return map[string]interface{}{
		"topic":         env.Topic,
		"hwId":          env.HwID,
		"traceId":       env.TraceID,
		"qos":           env.QoS,
		"retain":        env.Retain,
		"producedAt":    env.ProducedAt,
		"payloadBytes":  len(env.Payload),
		"payloadSha256": sha16,
	}
}

// AllowedTopics returns the allowlist as a sorted slice — used by
// tests and any future admin endpoint that wants to surface the live
// allowlist.
func AllowedTopics() []string {
	out := make([]string, 0, len(allowedTopics))
	for t := range allowedTopics {
		out = append(out, t)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
