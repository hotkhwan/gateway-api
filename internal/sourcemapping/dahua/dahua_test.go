// internal/sourcemapping/dahua/dahua_test.go
package dahua

import "testing"

// ev wraps a single Events[0].Data map in the Dahua envelope shape.
func ev(data map[string]any) map[string]any {
	return map[string]any{
		"Events": []any{map[string]any{"Action": "Pulse", "Code": "TrafficJunction", "Data": data}},
	}
}

func TestEventTypeFromPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"name VehicleDetect", ev(map[string]any{"Name": "VehicleDetect", "Vehicle": map[string]any{"ObjectType": "Vehicle"}}), "vehicle.detected"},
		{"name FaceDetect → captured", ev(map[string]any{"Name": "FaceDetect"}), "face.captured"},
		{"name PedestrianDetect", ev(map[string]any{"Name": "PedestrianDetect"}), "pedestrian.detected"},
		{"name NonMotorDetect (before vehicle)", ev(map[string]any{"Name": "NonMotorVehicleDetect"}), "non-motor-vehicle.detected"},
		{"generic name VM-3 → fallback to Vehicle group", ev(map[string]any{"Name": "VM-3", "TrafficCar": map[string]any{"VehicleColor": "Black"}}), "vehicle.detected"},
		{"generic name → Face group", ev(map[string]any{"Name": "Scene1", "Face": map[string]any{"id": 1}}), "face.captured"},
		{"generic name → non-motor group", ev(map[string]any{"Name": "X", "NonMotor": map[string]any{"k": "v"}}), "non-motor-vehicle.detected"},
		{"null object groups → fallback", ev(map[string]any{"Name": "VM-3", "NonMotorFeature": nil, "Vehicle": map[string]any{}}), "dahua.event"},
		{"no Events → fallback", map[string]any{"Ack": true}, "dahua.event"},
		{"empty Events → fallback", map[string]any{"Events": []any{}}, "dahua.event"},
		{"nil payload → fallback", nil, "dahua.event"},
	}
	for _, c := range cases {
		if got := EventTypeFromPayload(c.payload, "dahua.event"); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestPictureCoordinates(t *testing.T) {
	// Real-shape payload: vehicle + object boxes on the 0..8191 grid, scene 1920x1080.
	payload := map[string]any{"Events": []any{map[string]any{"Data": map[string]any{
		"SceneImage": map[string]any{"Width": float64(1920), "Height": float64(1080)},
		"Vehicle":    map[string]any{"BoundingBox": []any{float64(230), float64(659), float64(1954), float64(3552)}},
		"Object":     map[string]any{"BoundingBox": []any{float64(866), float64(2291), float64(1161), float64(2506)}},
	}}}}

	coords := PictureCoordinates(payload)
	if len(coords) != 2 {
		t.Fatalf("want 2 boxes (vehicle, object), got %d", len(coords))
	}
	veh, _ := coords[0].(map[string]any)
	if veh["width"].(float64) != 1920 || veh["height"].(float64) != 1080 {
		t.Errorf("scene dims wrong: %v", veh)
	}
	// 230/8192 ≈ 0.0281 ; values must be normalized into [0,1].
	for _, k := range []string{"x1", "y1", "x2", "y2"} {
		v := veh[k].(float64)
		if v < 0 || v > 1 {
			t.Errorf("vehicle %s=%v not in [0,1]", k, v)
		}
	}
	if got := veh["x1"].(float64); got < 0.027 || got > 0.029 {
		t.Errorf("vehicle x1 = %v, want ≈0.0281 (230/8192)", got)
	}
}

func TestPictureCoordinates_human(t *testing.T) {
	// HumanTrait carries the person box under HumanAttributes.BoundingBox.
	payload := map[string]any{"Events": []any{map[string]any{"Data": map[string]any{
		"SceneImage":      map[string]any{"Width": float64(1920), "Height": float64(1080)},
		"HumanAttributes": map[string]any{"BoundingBox": []any{float64(175), float64(1926), float64(1570), float64(8185)}},
	}}}}
	coords := PictureCoordinates(payload)
	if len(coords) != 1 {
		t.Fatalf("want 1 human box, got %d", len(coords))
	}
	b := coords[0].(map[string]any)
	// 175/8192 ≈ 0.0214 ; 8185/8192 ≈ 0.999 (clamped ≤ 1).
	if x1 := b["x1"].(float64); x1 < 0.020 || x1 > 0.023 {
		t.Errorf("human x1 = %v, want ≈0.0214", x1)
	}
	if y2 := b["y2"].(float64); y2 < 0 || y2 > 1 {
		t.Errorf("human y2 = %v not in [0,1]", y2)
	}
}

func TestPictureCoordinates_emptyWhenNoBox(t *testing.T) {
	if c := PictureCoordinates(map[string]any{"Ack": true}); c != nil {
		t.Errorf("want nil for non-Dahua payload, got %v", c)
	}
	// Event with no detection boxes → nil.
	p := map[string]any{"Events": []any{map[string]any{"Data": map[string]any{"Name": "VehicleDetect"}}}}
	if c := PictureCoordinates(p); c != nil {
		t.Errorf("want nil when no BoundingBox, got %v", c)
	}
}
