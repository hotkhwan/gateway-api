// internal/kafka/kctrlpubcons/consumer_test.go
//
// Per docs/contracts/kcontrol-outbound-commands.md §11 consumer-side
// unit tests. Covers:
//   - topic allowlist rejects unknown topic (no MQTT publish)
//   - payload size cap rejects oversize envelope
//   - shadow mode skips MQTT publish (default GW_KCONTROL_PUBLISH_ENABLED=false)
//   - live mode publishes with correct topic/qos/retain/payload
//   - allowlist membership pins contract §5.1.1
//   - log redaction: payload string NEVER appears in captured output
package kctrlpubcons

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
)

type publishCall struct {
	topic   string
	qos     byte
	retain  bool
	payload []byte
}

type fixture struct {
	mu         sync.Mutex
	publishes  []publishCall
	publishErr error
	prevEnv    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{prevEnv: os.Getenv("GW_KCONTROL_PUBLISH_ENABLED")}
	publishRawFn = func(topic string, qos byte, retain bool, payload []byte) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		// Defensive copy because the upstream consumer keeps the
		// envelope alive on the heap; tests must not observe a buffer
		// that gets recycled out from under them.
		buf := make([]byte, len(payload))
		copy(buf, payload)
		f.publishes = append(f.publishes, publishCall{topic, qos, retain, buf})
		return f.publishErr
	}
	t.Cleanup(func() {
		_ = os.Setenv("GW_KCONTROL_PUBLISH_ENABLED", f.prevEnv)
	})
	return f
}

func (f *fixture) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.publishes)
}

// ---- tests -----------------------------------------------------------------

func TestHandleEnvelope_RejectsUnknownTopic(t *testing.T) {
	f := newFixture(t)
	_ = os.Setenv("GW_KCONTROL_PUBLISH_ENABLED", "true")

	err := handleEnvelope(context.Background(), CommandEnvelope{
		Topic:   "kcontrol.evil",
		HwID:    "AA:BB",
		Payload: "{}",
	})
	if err != nil {
		t.Fatalf("handler returned err for unknown topic: %v", err)
	}
	if f.count() != 0 {
		t.Errorf("unknown topic MUST NOT publish; got %d publishes", f.count())
	}
}

func TestHandleEnvelope_RejectsOversizePayload(t *testing.T) {
	f := newFixture(t)
	_ = os.Setenv("GW_KCONTROL_PUBLISH_ENABLED", "true")

	big := strings.Repeat("x", payloadMaxBytes+1)
	err := handleEnvelope(context.Background(), CommandEnvelope{
		Topic:   "kcontrol.control",
		HwID:    "AA:BB",
		Payload: big,
	})
	if err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	if f.count() != 0 {
		t.Errorf("oversize payload MUST NOT publish; got %d", f.count())
	}
}

func TestHandleEnvelope_ShadowModeSkipsPublish(t *testing.T) {
	f := newFixture(t)
	_ = os.Setenv("GW_KCONTROL_PUBLISH_ENABLED", "false")

	err := handleEnvelope(context.Background(), CommandEnvelope{
		Topic:   "kcontrol.control",
		HwID:    "AA:BB",
		Payload: "{}",
	})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if f.count() != 0 {
		t.Errorf("shadow mode MUST NOT publish; got %d", f.count())
	}
}

// Default (env unset) MUST behave like shadow mode — a missing env var
// is the safest of the two options at cutover time. Operator typo /
// pre-deploy state should never accidentally double-publish.
func TestHandleEnvelope_EnvUnsetDefaultsToShadow(t *testing.T) {
	f := newFixture(t)
	_ = os.Unsetenv("GW_KCONTROL_PUBLISH_ENABLED")

	_ = handleEnvelope(context.Background(), CommandEnvelope{
		Topic:   "kcontrol.control",
		HwID:    "AA:BB",
		Payload: "{}",
	})
	if f.count() != 0 {
		t.Errorf("env unset MUST default to shadow; got %d publishes", f.count())
	}
}

func TestHandleEnvelope_LiveModePublishesByteIdentical(t *testing.T) {
	f := newFixture(t)
	_ = os.Setenv("GW_KCONTROL_PUBLISH_ENABLED", "true")

	body := `{"hwId":"AA:BB","setHealthInterval":30}`
	err := handleEnvelope(context.Background(), CommandEnvelope{
		Topic:   "kcontrol.control",
		HwID:    "AA:BB",
		Payload: body,
		QoS:     1,
		Retain:  true,
	})
	if err != nil {
		t.Fatalf("publish err: %v", err)
	}
	if f.count() != 1 {
		t.Fatalf("want 1 publish, got %d", f.count())
	}
	got := f.publishes[0]
	if got.topic != "kcontrol.control" {
		t.Errorf("topic wrong: %q", got.topic)
	}
	if got.qos != 1 {
		t.Errorf("qos wrong: %d", got.qos)
	}
	if !got.retain {
		t.Errorf("retain wrong")
	}
	if string(got.payload) != body {
		t.Errorf("payload byte-identity broken: got %q want %q", got.payload, body)
	}
}

// Allowlist must match producer-side allowlist exactly. If a topic is
// added on the producer side and forgotten here, the consumer will
// silently drop those commands — this test catches the drift.
func TestAllowlistMembershipPinsContract(t *testing.T) {
	want := map[string]bool{
		"kcontrol.control":     true,
		"kcontrol.alarmResult": true,
		"kcontrol/alarms":      true,
	}
	for _, topic := range AllowedTopics() {
		if !want[topic] {
			t.Errorf("unexpected allowed topic %q — contract §5.1.1 lock", topic)
		}
		delete(want, topic)
	}
	for missing := range want {
		t.Errorf("contract §5.1.1 topic %q missing from allowlist", missing)
	}
}

func TestAllowlistMustNotIncludeKcontrolHealth(t *testing.T) {
	for _, topic := range AllowedTopics() {
		if topic == "kcontrol.health" {
			t.Fatal("kcontrol.health was dropped per contract §3 audit; MUST NOT appear on allowlist (Codex round-1 blocker #4)")
		}
	}
}

// Redaction property: no log line emitted by handleEnvelope should
// include the raw payload string. The handler uses zerolog so we
// can't easily capture without a logger override — instead we test
// the redactedFields() function directly to confirm `payload` is
// absent and only safe metadata is present.
func TestRedactedFields_NoRawPayload(t *testing.T) {
	env := CommandEnvelope{
		Topic:   "kcontrol.control",
		HwID:    "AA:BB",
		Payload: `{"pass":"super-secret-mqtt-password","pem":"-----BEGIN PRIVATE KEY-----"}`,
	}
	fields := redactedFields(env)

	if _, present := fields["payload"]; present {
		t.Fatal("redactedFields MUST NOT include raw `payload` key (contract §8.1 — credentials leak risk)")
	}

	// Spot-check the redacted shape contains the documented fields.
	for _, key := range []string{"topic", "hwId", "traceId", "qos", "retain", "producedAt", "payloadBytes", "payloadSha256"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("redactedFields missing required key %q", key)
		}
	}

	// Spot-check no value (including stringified versions) leaks
	// the secret. We stringify the values and grep for "super-secret"
	// or "BEGIN PRIVATE KEY".
	for k, v := range fields {
		s := stringValue(v)
		if strings.Contains(s, "super-secret") || strings.Contains(s, "BEGIN PRIVATE KEY") {
			t.Errorf("field %q leaked credential substring: %q", k, s)
		}
	}
}

func stringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		// Cheap stringify for the int/bool fields — sufficient for
		// the "no credential leak" property; full reflection not needed.
		return ""
	}
}

func TestCommandsTopic_DefaultsAndOverride(t *testing.T) {
	prev := os.Getenv(envCommandsTopic)
	t.Cleanup(func() { _ = os.Setenv(envCommandsTopic, prev) })

	_ = os.Unsetenv(envCommandsTopic)
	if got := CommandsTopic(); got != defaultCommandsTopic {
		t.Errorf("default topic wrong: %q", got)
	}

	_ = os.Setenv(envCommandsTopic, "override.topic.v9")
	if got := CommandsTopic(); got != "override.topic.v9" {
		t.Errorf("env override not honored: %q", got)
	}
}
