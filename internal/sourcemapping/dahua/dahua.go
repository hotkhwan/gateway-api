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

// dahuaBoxGrid is the coordinate grid Dahua intelligent BoundingBoxes use
// (0..8191), used to normalize them into [0,1].
const dahuaBoxGrid = 8192.0

// PictureCoordinates derives AIBOX-style normalized detection boxes from a Dahua
// event. Each entry is {width,height,x1,y1,x2,y2} with x/y normalized to [0,1]
// (the raw BoundingBox is on a 0..8191 grid) and width/height the full scene
// dimensions — the same shape AIBOX emits. Returns the vehicle box then the
// object/plate box, whichever are present; nil when no box is found.
//
// The flat mapping template can't produce this (it needs per-element math), so
// the normalizer injects it for sourceFamily=="dahua". See normalizedcons.
func PictureCoordinates(payload map[string]any) []any {
	data := firstEventData(payload)
	if data == nil {
		return nil
	}
	w, h := sceneDims(data)
	var out []any
	for _, group := range []string{"Vehicle", "Object"} {
		if box := normalizedBox(asMap(data[group])["BoundingBox"], w, h); box != nil {
			out = append(out, box)
		}
	}
	return out
}

// sceneDims returns the full-frame width/height from SceneImage (default 1920x1080).
func sceneDims(data map[string]any) (float64, float64) {
	w, h := 1920.0, 1080.0
	if si := asMap(data["SceneImage"]); si != nil {
		if v := toFloat(si["Width"]); v > 0 {
			w = v
		}
		if v := toFloat(si["Height"]); v > 0 {
			h = v
		}
	}
	return w, h
}

// normalizedBox converts a Dahua [x1,y1,x2,y2] BoundingBox (0..8191 grid) into an
// AIBOX-style coordinate object. Returns nil for a missing or all-zero box.
func normalizedBox(v any, width, height float64) map[string]any {
	arr, ok := v.([]any)
	if !ok || len(arr) < 4 {
		return nil
	}
	x1, y1 := toFloat(arr[0])/dahuaBoxGrid, toFloat(arr[1])/dahuaBoxGrid
	x2, y2 := toFloat(arr[2])/dahuaBoxGrid, toFloat(arr[3])/dahuaBoxGrid
	if x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0 {
		return nil
	}
	return map[string]any{
		"width":  width,
		"height": height,
		"x1":     clamp01(x1),
		"y1":     clamp01(y1),
		"x2":     clamp01(x2),
		"y2":     clamp01(y2),
	}
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
