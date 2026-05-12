// internal/mqtt/kctrlsubmsg/handler_test.go
package kctrlsubmsg

import (
	"os"
	"testing"
	"time"
)

func TestKafkaTopicForMQTT_Defaults(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"kcontrol.health", "gw.kcontrol.health.v1"},
		{"kcontrol.alarms", "gw.kcontrol.alarms.v1"},
		{"kcontrol.sensor", "gw.kcontrol.sensor.v1"},
		{"kcontrol.events", "gw.kcontrol.events.v1"},
		{"kcontrol.response", "gw.kcontrol.response.v1"},
		{"kcontrol.unknown", ""},
		{"", ""},
	}
	// Make sure env overrides do not interfere with the default-path assertion.
	for _, k := range []string{
		"KAFKA_TOPIC_GW_KCONTROL_HEALTH",
		"KAFKA_TOPIC_GW_KCONTROL_ALARMS",
		"KAFKA_TOPIC_GW_KCONTROL_SENSOR",
		"KAFKA_TOPIC_GW_KCONTROL_EVENTS",
		"KAFKA_TOPIC_GW_KCONTROL_RESPONSE",
	} {
		_ = os.Unsetenv(k)
	}
	for _, c := range cases {
		if got := kafkaTopicForMQTT(c.in); got != c.want {
			t.Errorf("kafkaTopicForMQTT(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestKafkaTopicForMQTT_EnvOverride(t *testing.T) {
	t.Setenv("KAFKA_TOPIC_GW_KCONTROL_HEALTH", "custom.health.topic")
	t.Setenv("KAFKA_TOPIC_GW_KCONTROL_ALARMS", "custom.alarms.topic")

	if got := kafkaTopicForMQTT("kcontrol.health"); got != "custom.health.topic" {
		t.Errorf("env override for health failed: got %q", got)
	}
	if got := kafkaTopicForMQTT("kcontrol.alarms"); got != "custom.alarms.topic" {
		t.Errorf("env override for alarms failed: got %q", got)
	}
	// non-overridden topics keep defaults
	if got := kafkaTopicForMQTT("kcontrol.sensor"); got != "gw.kcontrol.sensor.v1" {
		t.Errorf("non-overridden default broken: got %q", got)
	}
}

func TestKindForMQTT(t *testing.T) {
	cases := map[string]string{
		"kcontrol.health":   "health",
		"kcontrol.alarms":   "alarms",
		"kcontrol.sensor":   "sensor",
		"kcontrol.events":   "events",
		"kcontrol.response": "response",
		"":                  "unknown",
		"foo.bar":           "unknown",
	}
	for in, want := range cases {
		if got := kindForMQTT(in); got != want {
			t.Errorf("kindForMQTT(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTimestamp_FiredAtPrecedence(t *testing.T) {
	now := time.Now().Unix()
	m := map[string]any{
		"firedAt":   float64(now),
		"startedAt": float64(now - 100),
		"timestamp": float64(now - 200),
	}
	got := parseTimestamp(m).Unix()
	if got != now {
		t.Errorf("firedAt should win precedence: got %d, want %d", got, now)
	}
}

func TestParseTimestamp_StartedAtFallback(t *testing.T) {
	now := time.Now().Unix()
	m := map[string]any{
		"startedAt": float64(now),
		"timestamp": float64(now - 200),
	}
	got := parseTimestamp(m).Unix()
	if got != now {
		t.Errorf("startedAt should be used when firedAt missing: got %d, want %d", got, now)
	}
}

func TestParseTimestamp_RFC3339String(t *testing.T) {
	m := map[string]any{
		"timestamp": "2026-05-12T10:00:00Z",
	}
	got := parseTimestamp(m).UTC().Format(time.RFC3339)
	if got != "2026-05-12T10:00:00Z" {
		t.Errorf("RFC3339 string timestamp not parsed: got %q", got)
	}
}

func TestParseTimestamp_FallbackToNow(t *testing.T) {
	before := time.Now().Unix()
	got := parseTimestamp(map[string]any{}).Unix()
	after := time.Now().Unix()
	if got < before || got > after+1 {
		t.Errorf("fallback timestamp out of range: got %d, before %d, after %d", got, before, after)
	}
}

func TestToInt64_Variants(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"float64", float64(42), 42, true},
		{"int", int(42), 42, true},
		{"int64", int64(42), 42, true},
		{"string", "42", 42, true},
		{"empty string", "", 0, false},
		{"nil", nil, 0, false},
		{"struct", struct{}{}, 0, false},
	}
	for _, c := range cases {
		got, ok := toInt64(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: toInt64(%v) = (%d, %v), want (%d, %v)", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestStrOrEmpty(t *testing.T) {
	if got := strOrEmpty("hello"); got != "hello" {
		t.Errorf("string value not returned: got %q", got)
	}
	if got := strOrEmpty(42); got != "" {
		t.Errorf("non-string should yield empty: got %q", got)
	}
	if got := strOrEmpty(nil); got != "" {
		t.Errorf("nil should yield empty: got %q", got)
	}
}
