// internal/kafka/normalizedcons/bridgePayload_test.go
package normalizedcons

import (
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// Regression: the bridge event (gw.events.normalized.v1) must carry the full
// normalized payload when the template maps flat fields (no "eventAttribute"
// sub-key), not an empty object. Dahua ANPR templates map plate/vehicleColor/…
// directly.
func TestBuildBridgeEvent_FlatPayloadForwarded(t *testing.T) {
	event := &ingestmod.NormalizedEvent{
		EventId:   "e1",
		EventType: "dahua.event",
		Payload: map[string]any{
			"vehicleColor": "Black",
			"plate":        "1กข2345",
			"eventCode":    "TrafficJunction",
		},
	}
	b := buildBridgeEvent(event, "ws", "org", "tr", "", "dahua", map[string]any{})
	if b.Payload["vehicleColor"] != "Black" || b.Payload["plate"] != "1กข2345" || b.Payload["eventCode"] != "TrafficJunction" {
		t.Fatalf("flat payload not forwarded to bridge: %v", b.Payload)
	}
}

// When an "eventAttribute" sub-key is present (legacy AIBOX-shaped), the bridge
// uses it and does not leak sibling keys.
func TestBuildBridgeEvent_EventAttributeUsed(t *testing.T) {
	event := &ingestmod.NormalizedEvent{
		EventType: "x.event",
		Payload: map[string]any{
			"eventAttribute": map[string]any{"k": "v"},
			"sibling":        "ignored",
		},
	}
	b := buildBridgeEvent(event, "ws", "org", "tr", "", "x", map[string]any{})
	if b.Payload["k"] != "v" {
		t.Fatalf("eventAttribute not used: %v", b.Payload)
	}
	if _, ok := b.Payload["sibling"]; ok {
		t.Fatalf("sibling key leaked when eventAttribute present: %v", b.Payload)
	}
}

// The flat-payload forward must copy, not alias event.Payload — the
// pictureCoordinates write must not mutate the source.
func TestBuildBridgeEvent_NoAlias(t *testing.T) {
	event := &ingestmod.NormalizedEvent{Payload: map[string]any{"a": 1}}
	b := buildBridgeEvent(event, "ws", "org", "tr", "", "dahua", map[string]any{"pictureCoordinates": []int{1, 2}})
	if _, ok := event.Payload["pictureCoordinates"]; ok {
		t.Fatalf("event.Payload was aliased/mutated: %v", event.Payload)
	}
	if _, ok := b.Payload["pictureCoordinates"]; !ok {
		t.Fatalf("bridge should carry pictureCoordinates from rawPayload: %v", b.Payload)
	}
}
