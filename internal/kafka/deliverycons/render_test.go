// internal/kafka/deliverycons/render_test.go
package deliverycons

import (
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func sampleIngestmodForRender() *ingestmod.NormalizedEvent {
	return &ingestmod.NormalizedEvent{
		EventId:       "evt-1",
		TenantId:      "org-1",
		EventType:     "non-motor-vehicle.detected",
		EventCategory: "non-motor-vehicle",
		EventAction:   "detected",
		EventClass:    "alert",
		EventSeverity: "low",
		OccurredAt:    time.Date(2026, 4, 21, 6, 53, 12, 0, time.UTC),
		Source: ingestmod.SourceInfo{
			DeviceId:    "51",
			DeviceType:  "AIBOX",
			SubType:     "non-motor-vehicle.detected",
			WorkspaceId: "ws-1",
		},
		Payload: map[string]any{"helmet": 2, "number": 2},
	}
}

// TestRenderContext_AllLegacyKeysPresent guards against silent rename or
// removal of any existing template variable. Template authors rely on these
// exact names; changing them breaks every saved MessageTemplate.
func TestRenderContext_AllLegacyKeysPresent(t *testing.T) {
	ctx := renderContext(sampleIngestmodForRender())

	legacyKeys := []string{
		"eventId", "tenantId", "eventType", "eventCategory", "eventAction",
		"eventClass", "eventSeverity", "sourceFamily", "occurredAt", "payload",
		"source", "location", "geo",
	}
	for _, k := range legacyKeys {
		if _, ok := ctx[k]; !ok {
			t.Errorf("legacy key %q missing from render context — template authors depend on this", k)
		}
	}

	// Spot-check a few values to ensure semantics preserved.
	if ctx["eventAction"] != "detected" {
		t.Errorf(".eventAction = %v, want \"detected\"", ctx["eventAction"])
	}
	if ctx["sourceFamily"] != "AIBOX" {
		t.Errorf(".sourceFamily = %v, want \"AIBOX\"", ctx["sourceFamily"])
	}
	src, ok := ctx["source"].(map[string]any)
	if !ok {
		t.Fatal(".source not a map")
	}
	if src["workspaceId"] != "ws-1" {
		t.Errorf(".source.workspaceId = %v, want \"ws-1\"", src["workspaceId"])
	}
}

// TestRenderContext_LegacyKeysRenderIdentically verifies that after the
// Wide slice removed the .canonical namespace, legacy keys still render
// the same values — template authors depending on .eventAction etc. keep
// working unchanged.
func TestRenderContext_LegacyKeysRenderIdentically(t *testing.T) {
	ctx := renderContext(sampleIngestmodForRender())

	if out, err := renderText("{{.eventAction}}", ctx); err != nil {
		t.Errorf("render {{.eventAction}}: %v", err)
	} else if out != "detected" {
		t.Errorf("{{.eventAction}} = %q, want \"detected\"", out)
	}

	if out, err := renderText("{{.sourceFamily}}", ctx); err != nil {
		t.Errorf("render {{.sourceFamily}}: %v", err)
	} else if out != "AIBOX" {
		t.Errorf("{{.sourceFamily}} = %q, want \"AIBOX\"", out)
	}
}

// TestRenderContext_CanonicalBlockRemoved pins that the .canonical namespace
// no longer exists in the render context (narrow-v2 retirement at Phase-E).
func TestRenderContext_CanonicalBlockRemoved(t *testing.T) {
	ctx := renderContext(sampleIngestmodForRender())
	if _, ok := ctx["canonical"]; ok {
		t.Error("render context should not expose .canonical after Wide Phase-E")
	}
}
