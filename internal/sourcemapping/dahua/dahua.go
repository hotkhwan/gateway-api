// internal/sourcemapping/dahua/dahua.go
package dahua

import "strings"

// EventTypeFromPayload derives the canonical "{category}.{action}" eventType for
// a Dahua event so it shares the AIBOX taxonomy (see sourcemapping/aibox):
//
//	category ∈ vehicle | non-motor-vehicle | pedestrian | face
//	action   ∈ detected (default) | captured (face)
//
// Examples: vehicle.detected, non-motor-vehicle.detected, pedestrian.detected,
// face.captured. splitEventType (normalizer) then yields sourceCategory/sourceAction.
//
// Primary signal is Events[0].Data.Name (e.g. "VehicleDetect"); when the rule is
// named generically (e.g. a scene name like "VM-3") it falls back to which
// detected-object group is present under Events[0].Data. Returns fallback when
// nothing matches (e.g. heartbeat / unknown event).
func EventTypeFromPayload(payload map[string]any, fallback string) string {
	data := firstEventData(payload)
	if data == nil {
		return fallback
	}
	cat := categoryFromData(data)
	if cat == "" {
		return fallback
	}
	action := "detected"
	if cat == "face" {
		action = "captured"
	}
	return cat + "." + action
}

// firstEventData returns Events[0].Data, or nil when the payload is not a
// Dahua multi-event envelope.
func firstEventData(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	events, ok := payload["Events"].([]any)
	if !ok || len(events) == 0 {
		return nil
	}
	ev, ok := events[0].(map[string]any)
	if !ok {
		return nil
	}
	data, _ := ev["Data"].(map[string]any)
	return data
}

// categoryFromData prefers the rule Name keyword, then the presence of a
// detected-object group.
func categoryFromData(data map[string]any) string {
	if c := categoryFromName(asString(data["Name"])); c != "" {
		return c
	}
	switch {
	case present(data, "Face"):
		return "face"
	case present(data, "NonMotor"), present(data, "NonMotorFeature"), present(data, "NonMotorVehicle"):
		return "non-motor-vehicle"
	case present(data, "Human"), present(data, "Pedestrian"):
		return "pedestrian"
	case present(data, "Vehicle"), present(data, "TrafficCar"):
		return "vehicle"
	}
	return ""
}

// categoryFromName maps a Dahua rule Name to a canonical category.
// Order matters: non-motor before vehicle (its keyword contains "vehicle").
func categoryFromName(name string) string {
	n := strings.ToLower(name)
	switch {
	case n == "":
		return ""
	case strings.Contains(n, "face"):
		return "face"
	case strings.Contains(n, "nonmotor"), strings.Contains(n, "non-motor"):
		return "non-motor-vehicle"
	case strings.Contains(n, "pedestrian"), strings.Contains(n, "human"):
		return "pedestrian"
	case strings.Contains(n, "vehicle"), strings.Contains(n, "traffic"), strings.Contains(n, "car"):
		return "vehicle"
	}
	return ""
}

// present reports whether key holds a non-nil, non-empty value (Dahua emits
// absent object groups as null or empty objects).
func present(data map[string]any, key string) bool {
	v, ok := data[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case map[string]any:
		return len(t) > 0
	case []any:
		return len(t) > 0
	default:
		return true
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
