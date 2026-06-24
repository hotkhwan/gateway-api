// internal/services/ingestsvc/mappingArray_test.go
package ingestsvc

import (
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// dahuaRaw mirrors the shape a Dahua TrafficJunction (ANPR) event has after the
// multipart parser merges the metadata part: fields nest under the Events array.
func dahuaRaw() map[string]any {
	return map[string]any{
		"Channel": float64(0),
		"Events": []any{
			map[string]any{
				"Action": "Pulse",
				"Code":   "TrafficJunction",
				"Data": map[string]any{
					"RealUTC": float64(1771424063),
					"TrafficCar": map[string]any{
						"VehicleColor": "Black",
						"PlateNumber":  "1กข2345",
						"MachineName":  "Dahua001",
					},
				},
			},
		},
	}
}

func TestGetNestedValue_ArrayIndex(t *testing.T) {
	raw := dahuaRaw()
	cases := []struct {
		path string
		want any
		ok   bool
	}{
		{"Events.0.Code", "TrafficJunction", true},
		{"Events.0.Data.TrafficCar.VehicleColor", "Black", true},
		{"Events.0.Data.TrafficCar.PlateNumber", "1กข2345", true},
		{"Events.0.Data.RealUTC", float64(1771424063), true},
		{"Channel", float64(0), true},
		{"Events.1.Code", nil, false},    // index out of range
		{"Events.x.Code", nil, false},    // non-numeric index into array
		{"Events.0.Missing", nil, false}, // missing key
		{"Events.0.Data.TrafficCar.X", nil, false},
	}
	for _, c := range cases {
		got, ok := getNestedValue(raw, c.path)
		if ok != c.ok {
			t.Fatalf("%s: ok=%v want %v", c.path, ok, c.ok)
		}
		if c.ok && c.want != nil && got != c.want {
			t.Fatalf("%s: got %v want %v", c.path, got, c.want)
		}
	}

	// A mid-path segment that resolves to a non-leaf map is still ok.
	if v, ok := getNestedValue(raw, "Events.0.Data.TrafficCar"); !ok {
		t.Fatal("expected TrafficCar map resolvable")
	} else if _, isMap := v.(map[string]any); !isMap {
		t.Fatalf("expected map, got %T", v)
	}
}

func TestApplyMappings_DahuaArrayPaths(t *testing.T) {
	raw := dahuaRaw()
	mappings := []ingestmod.FieldMapping{
		{TargetPath: "vehicleColor", SourcePath: "Events.0.Data.TrafficCar.VehicleColor"},
		{TargetPath: "plate", SourcePath: "Events.0.Data.TrafficCar.PlateNumber"},
		{TargetPath: "eventCode", SourcePath: "Events.0.Code"},
		{TargetPath: "occurredAt", SourcePath: "Events.0.Data.RealUTC", Transform: "timestamp"},
		{TargetPath: "missingThing", SourcePath: "Events.0.Data.TrafficCar.PlateColor", Required: true},
	}

	// ApplyMappings uses no repo/log state — zero value is fine.
	m := &TemplateMatcher{}
	mapped, missing := m.ApplyMappings(raw, mappings)

	if mapped["vehicleColor"] != "Black" {
		t.Fatalf("vehicleColor: got %v", mapped["vehicleColor"])
	}
	if mapped["plate"] != "1กข2345" {
		t.Fatalf("plate: got %v", mapped["plate"])
	}
	if mapped["eventCode"] != "TrafficJunction" {
		t.Fatalf("eventCode: got %v", mapped["eventCode"])
	}
	if ts, ok := mapped["occurredAt"].(time.Time); !ok {
		t.Fatalf("occurredAt: not time.Time, got %T", mapped["occurredAt"])
	} else if ts.Unix() != 1771424063 {
		t.Fatalf("occurredAt: got %d want 1771424063", ts.Unix())
	}
	// PlateColor is absent (null in real events) → required-missing is reported,
	// not silently mapped.
	if len(missing) != 1 || missing[0] != "missingThing" {
		t.Fatalf("missing: got %v want [missingThing]", missing)
	}
}

func TestPlausibleEventTime(t *testing.T) {
	ref := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"same", ref, true},
		{"1h ago", ref.Add(-time.Hour), true},
		{"47h ago", ref.Add(-47 * time.Hour), true},
		{"49h ago (too old)", ref.Add(-49 * time.Hour), false},
		{"30m ahead", ref.Add(30 * time.Minute), true},
		{"2h ahead (clock-ahead)", ref.Add(2 * time.Hour), false},
		{"camera clock months off", time.Date(2026, 2, 18, 14, 55, 0, 0, time.UTC), false},
		{"zero", time.Time{}, false},
	}
	for _, c := range cases {
		if got := plausibleEventTime(c.t, ref); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
