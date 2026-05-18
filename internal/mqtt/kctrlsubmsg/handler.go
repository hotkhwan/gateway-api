// internal/mqtt/kctrlsubmsg/handler.go
package kctrlsubmsg

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/kctrlregistrysvc"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// registryDecider is the narrow interface the MQTT handler needs from
// kctrlregistrysvc.Service. Allows tests to substitute a fake without standing
// up Mongo + the LRU cache.
type registryDecider interface {
	Decide(ctx context.Context, hwId string) kctrlregistrysvc.Decision
}

// deciderHolder boxes the registryDecider so atomic.Value stores a single
// concrete pointer type across nil and non-nil values — atomic.Value
// requires consistent dynamic type across Stores, which a bare interface
// can't satisfy when alternating between nil and a typed pointer.
type deciderHolder struct{ d registryDecider }

// deciderRef is the process-global registry decision source set by main /
// container at boot. nil holder (or unset) = legacy "forward verbatim"
// behavior (the pre-Phase A baseline). Set via SetRegistryDecider to enable
// contract §5 3-branch routing.
var deciderRef atomic.Value // holds *deciderHolder

// SetRegistryDecider installs the registry decision source used by
// MessageHandler. Called once from the container at boot. Idempotent — a nil
// argument is treated as "disable" (revert to legacy forward-verbatim).
func SetRegistryDecider(d registryDecider) {
	deciderRef.Store(&deciderHolder{d: d})
}

// loadDecider returns the currently-installed registry decider, or nil if
// none has been wired. Reads are lock-free via atomic.Value.
func loadDecider() registryDecider {
	v := deciderRef.Load()
	if v == nil {
		return nil
	}
	h, ok := v.(*deciderHolder)
	if !ok || h == nil {
		return nil
	}
	return h.d
}

// kafkaTopicForMQTT maps an inbound MQTT topic to the corresponding outbound
// Kafka topic (env-driven, gw-prefixed, v1). Returned topic is empty for
// unknown MQTT topics — caller must skip.
func kafkaTopicForMQTT(mqttTopic string) string {
	switch mqttTopic {
	case "kcontrol.health":
		return config.TopicEnv("KAFKA_TOPIC_GW_KCONTROL_HEALTH", "gw.kcontrol.health.v1")
	case "kcontrol.alarms":
		return config.TopicEnv("KAFKA_TOPIC_GW_KCONTROL_ALARMS", "gw.kcontrol.alarms.v1")
	case "kcontrol.sensor":
		return config.TopicEnv("KAFKA_TOPIC_GW_KCONTROL_SENSOR", "gw.kcontrol.sensor.v1")
	case "kcontrol.events":
		return config.TopicEnv("KAFKA_TOPIC_GW_KCONTROL_EVENTS", "gw.kcontrol.events.v1")
	case "kcontrol.response":
		return config.TopicEnv("KAFKA_TOPIC_GW_KCONTROL_RESPONSE", "gw.kcontrol.response.v1")
	}
	return ""
}

// kindForMQTT derives the human-readable `kind` field for the canonical
// Kafka envelope. The same names are used in the legacy klynx-api
// kcontrolmsg envelope so downstream klynx-api consumers can switch source
// without re-mapping payload shape.
func kindForMQTT(mqttTopic string) string {
	switch mqttTopic {
	case "kcontrol.health":
		return "health"
	case "kcontrol.alarms":
		return "alarms"
	case "kcontrol.sensor":
		return "sensor"
	case "kcontrol.events":
		return "events"
	case "kcontrol.response":
		return "response"
	}
	return "unknown"
}

// MessageHandler is the paho subscriber callback. It is intentionally a
// pure MQTT→Kafka forwarder:
//
//   - parse JSON payload
//   - build canonical envelope { topic, hwId, timestamp, kind, bridge, ...src }
//   - publish to the gw.kcontrol.*.v1 Kafka topic for the source MQTT topic
//
// Domain projection (Mongo upsert, approved-device filter) is the
// responsibility of the downstream klynx-api consumers (Phase 2 —
// internal/kafka/gwkctrl*cons/). Keeping projection out of this handler
// preserves the "gateway-api owns ingestion boundary; klynx-api owns Klynx
// projection store" split in the Phase 0 plan.
func MessageHandler(_ mqtt.Client, msg mqtt.Message) {
	log := logger.WithMeta("kctrlsubmsg", "MessageHandler")

	mqttTopic := msg.Topic()
	payload := msg.Payload()

	// Retained-clear sentinel.
	if len(payload) == 0 || string(payload) == `""` {
		log.Debug().Str("topic", mqttTopic).Msg("🟡 Empty MQTT payload (retained cleared), skip")
		return
	}

	// Retained alarms can be re-played on subscribe / reconnect — same skip
	// rule the legacy klynx-api kcontrolmsg subscriber uses so behaviour
	// stays observably identical during the dual-publish window.
	if msg.Retained() && mqttTopic == "kcontrol.alarms" {
		log.Debug().Str("topic", mqttTopic).Msg("🟡 Retained alarms, skip")
		return
	}

	kafkaTopic := kafkaTopicForMQTT(mqttTopic)
	if kafkaTopic == "" {
		log.Warn().Str("topic", mqttTopic).Msg("⚠️ Unknown MQTT topic, no Kafka target — skip")
		return
	}

	// --------- Parse payload → map ----------
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		log.Warn().Err(err).Str("topic", mqttTopic).RawJSON("payload", payload).Msg("⚠️ Invalid MQTT payload — skip")
		return
	}

	// hwId is the routing key. mac is the legacy fallback used by older
	// firmware that publishes without hwId.
	hwID := strOrEmpty(m["hwId"])
	if hwID == "" {
		if mac := strOrEmpty(m["mac"]); mac != "" {
			hwID = mac
		}
	}
	if hwID == "" {
		log.Warn().Str("topic", mqttTopic).RawJSON("payload", payload).Msg("⚠️ Missing hwId — forwarding with sha1 key")
	}

	kind := kindForMQTT(mqttTopic)
	ts := parseTimestamp(m)

	// --------- Build canonical envelope ----------
	// Schema matches the legacy klynx-api kcontrolmsg envelope so Phase 2
	// klynx-api consumers can use the same parse shape.
	out := map[string]any{
		"topic":     mqttTopic,
		"hwId":      hwID,
		"timestamp": ts.UTC().Format(time.RFC3339),
		"kind":      kind,
		"bridge":    os.Getenv("POD_NAME"),
	}
	for k, v := range m {
		switch k {
		case "topic", "hwId", "timestamp", "kind":
			continue
		default:
			out[k] = v
		}
	}

	key := hwID
	if key == "" {
		h := sha1.Sum(payload)
		key = fmt.Sprintf("%x", h)
	}

	headers := map[string]string{
		"event":  "gw.kcontrol.received",
		"schema": "kcontrol/envelope/3",
		"kind":   kind,
		"bridge": os.Getenv("POD_NAME"),
	}

	ctx := context.Background()

	// Contract §5 — 3-branch routing against kctrl_registry. When no decider
	// is wired (test / bootstrap before container ready) we fall through to
	// the legacy forward-as-is behavior.
	if dec := loadDecider(); dec != nil && hwID != "" {
		d := dec.Decide(ctx, hwID)
		switch d.Action {
		case kctrlregistrysvc.ActionEnrich:
			// Approved device — enrich envelope with the operator-resolved
			// orgId + workspaceId so klynx-api consumers can broadcast
			// without the Layer-1 Mongo fallback.
			out["orgId"] = d.OrgId
			if d.WorkspaceId != "" {
				out["workspaceId"] = d.WorkspaceId
			}
			headers["orgId"] = d.OrgId
			log.Debug().
				Str("hwId", hwID).
				Str("orgId", d.OrgId).
				Str("mqttTopic", mqttTopic).
				Msg("kctrlsubmsg: enriched")
		case kctrlregistrysvc.ActionDrop:
			// Operator explicitly revoked this device. Do not write to Kafka.
			log.Info().
				Str("hwId", hwID).
				Str("mqttTopic", mqttTopic).
				Msg("kctrlsubmsg: hwId revoked — dropping")
			return
		case kctrlregistrysvc.ActionForward:
			// Unknown device — forward verbatim so klynx-api can discover it
			// per contract §5.1. orgId stays empty; klynx Layer-1 resolver
			// (4.59.2) handles enrichment on the consumer side until backfill
			// or admin approve fills the registry.
			log.Info().
				Str("hwId", hwID).
				Str("mqttTopic", mqttTopic).
				Msg("kctrlsubmsg: unknown hwId — forwarding for discovery")
		}
	}


	if err := kafka.PublishEventTo(ctx, kafkaTopic, key, out, headers); err != nil {
		log.Error().
			Err(err).
			Str("mqttTopic", mqttTopic).
			Str("kafkaTopic", kafkaTopic).
			Str("hwId", hwID).
			Msg("❌ Kafka publish failed")
		return
	}

	// Health forwards run every device-tick — keep info-log noise low.
	if kind == "health" {
		log.Debug().
			Str("hwId", hwID).
			Str("mqttTopic", mqttTopic).
			Str("kafkaTopic", kafkaTopic).
			Msg("✅ MQTT→Kafka forwarded")
	} else {
		log.Info().
			Str("hwId", hwID).
			Str("mqttTopic", mqttTopic).
			Str("kafkaTopic", kafkaTopic).
			Msg("✅ MQTT→Kafka forwarded")
	}
}

// parseTimestamp walks the standard kcontrol timestamp fields in the order
// produced by current firmware. Returns time.Now() if none match.
func parseTimestamp(m map[string]any) time.Time {
	if v, ok := toInt64(m["firedAt"]); ok && v > 0 {
		return time.Unix(v, 0)
	}
	if v, ok := toInt64(m["startedAt"]); ok && v > 0 {
		return time.Unix(v, 0)
	}
	if v, ok := toInt64(m["timestamp"]); ok && v > 0 {
		return time.Unix(v, 0)
	}
	if s, ok := m["timestamp"].(string); ok && strings.TrimSpace(s) != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			return parsed
		}
	}
	return time.Now()
}

// ===== helpers =====

func strOrEmpty(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i, true
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}
