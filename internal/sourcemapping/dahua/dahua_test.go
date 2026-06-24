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
