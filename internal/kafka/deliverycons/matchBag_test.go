// internal/kafka/deliverycons/matchBag_test.go
package deliverycons

import (
	"strings"
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func fixtureEvent() *ingestmod.NormalizedEvent {
	return &ingestmod.NormalizedEvent{
		EventId:       "evt-1",
		TenantId:      "ten-1",
		EventType:     "non-motor-vehicle.detected",
		EventCategory: "non-motor-vehicle",
		EventAction:   "detected",
		EventClass:    "alert",
		EventSeverity: "low",
		OccurredAt:    time.Now().UTC(),
		Source: ingestmod.SourceInfo{
			DeviceId:    "51",
			DeviceType:  "camera",
			DeviceName:  "front-gate",
			Vendor:      "AIBOX",
			Protocol:    "http",
			WorkspaceId: "ws-1",
		},
		Location: ingestmod.LocationInfo{Lat: 7.9, Lng: 98.3, Zone: "PHK"},
		Geo: ingestmod.GeoInfo{
			CountryCode: "TH",
			AdminLevel:  1,
			AdminName:   "Phuket",
			AdminCode:   "TH-83",
			IdScheme:    "ISO_3166_2",
		},
		GeoCell: ingestmod.GeoCellInfo{Scheme: "geohash", Precision: 5, Cell: "w1mvn"},
		Payload: map[string]any{
			"action":     "captured",
			"helmet":     1,
			"pictureUrl": "s3://canonical/...",
		},
		Meta: ingestmod.NormalizationMeta{
			SchemaVersion: "v1",
			TraceId:       "trace-xyz",
			TemplateId:    "tmpl-abc",
		},
	}
}

func TestBuildDeliveryMatchBag_TopLevelCanonicalFields(t *testing.T) {
	bag := buildDeliveryMatchBag(fixtureEvent())

	required := []string{
		"eventId", "tenantId", "eventType", "eventCategory", "eventAction",
		"eventClass", "eventSeverity", "occurredAt",
		"sourceType", "sourceCategory", "sourceAction",
		"templateId", "workspaceId",
	}
	for _, k := range required {
		if _, ok := bag[k]; !ok {
			t.Errorf("expected bag[%q] to be present", k)
		}
	}
}

func TestBuildDeliveryMatchBag_SourceAliasesMapToEventFields(t *testing.T) {
	bag := buildDeliveryMatchBag(fixtureEvent())

	if bag["sourceAction"] != bag["eventAction"] {
		t.Errorf("sourceAction should alias eventAction; got %v vs %v", bag["sourceAction"], bag["eventAction"])
	}
	if bag["sourceCategory"] != bag["eventCategory"] {
		t.Errorf("sourceCategory should alias eventCategory")
	}
	if bag["sourceType"] != bag["eventType"] {
		t.Errorf("sourceType should alias eventType")
	}
}

func TestBuildDeliveryMatchBag_NestedNamespaces(t *testing.T) {
	bag := buildDeliveryMatchBag(fixtureEvent())

	src, ok := bag["source"].(map[string]any)
	if !ok {
		t.Fatal("source should be a nested map")
	}
	if src["deviceId"] != "51" {
		t.Errorf("source.deviceId = %v, want 51", src["deviceId"])
	}

	geo, ok := bag["geo"].(map[string]any)
	if !ok {
		t.Fatal("geo should be a nested map")
	}
	if geo["adminCode"] != "TH-83" {
		t.Errorf("geo.adminCode = %v, want TH-83", geo["adminCode"])
	}

	loc, ok := bag["location"].(map[string]any)
	if !ok {
		t.Fatal("location should be a nested map")
	}
	if loc["zone"] != "PHK" {
		t.Errorf("location.zone = %v, want PHK", loc["zone"])
	}

	payload, ok := bag["payload"].(map[string]any)
	if !ok {
		t.Fatal("payload should be a nested map")
	}
	if payload["action"] != "captured" {
		t.Errorf("payload.action = %v, want captured", payload["action"])
	}
}

func TestBuildDeliveryMatchBag_NoRawNamespace(t *testing.T) {
	bag := buildDeliveryMatchBag(fixtureEvent())

	for k := range bag {
		if k == "raw" || strings.HasPrefix(k, "raw.") {
			t.Errorf("delivery bag must not expose raw.* namespace; found key %q", k)
		}
	}
}

func TestBuildDeliveryMatchBag_NilEventSafe(t *testing.T) {
	bag := buildDeliveryMatchBag(nil)
	if bag == nil {
		t.Fatal("expected non-nil bag for nil event")
	}
	if len(bag) != 0 {
		t.Errorf("expected empty bag for nil event, got %d keys", len(bag))
	}
}

func TestBuildDeliveryMatchBag_NilPayloadYieldsEmptyMap(t *testing.T) {
	ev := fixtureEvent()
	ev.Payload = nil
	bag := buildDeliveryMatchBag(ev)
	payload, ok := bag["payload"].(map[string]any)
	if !ok {
		t.Fatal("payload should be a map even when event.Payload is nil")
	}
	if len(payload) != 0 {
		t.Errorf("expected empty payload map, got %d keys", len(payload))
	}
}
