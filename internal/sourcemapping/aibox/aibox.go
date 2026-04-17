// internal/sourcemapping/aibox/aibox.go
package aibox

// AlarmTypeEventType maps AIBOX alarmType integer codes to canonical eventType strings.
// Source: AIBOX API spec — Alarm Type table.
var AlarmTypeEventType = map[int]string{
	0:  "unknown",
	1:  "face.captured",
	2:  "pedestrian.detected",
	3:  "non-motor-vehicle.detected",
	4:  "vehicle.detected",
	7:  "electric-vehicle.elevator-entry.detected",
	9:  "non-motor-vehicle.illegal-parking.detected",
	11: "trash-pile.detected",
	12: "trash-bin.overflow.detected",
	13: "lingering.detected",
	14: "non-motor-vehicle.intrusion.detected",
	15: "crowd-gathering.detected",
	16: "fall.detected",
	17: "smoke.detected",
	18: "open-fire.detected",
	19: "no-helmet.detected",
	20: "high-visibility-clothing.not-wearing.detected",
	21: "intrusion.detected",
	22: "no-mask.detected",
	24: "mobile-phone.detected",
	25: "smoking.detected",
	26: "absenteeism.detected",
	27: "non-motor-vehicle.illegal-parking.detected",
	28: "traffic-congestion.detected",
	29: "shared-bicycle.illegal-parking.detected",
	30: "heavy-vehicle.detected",
	31: "prohibited-banner.detected",
	32: "electric-vehicle.no-helmet.detected",
	35: "motor-vehicle.intrusion.detected",
	36: "door-window.status.detected",
	39: "electric-vehicle.passenger.detected",
	40: "food-delivery-rider.detected",
	41: "pedestrian-traffic.statistic",
	42: "personnel.stay-duration",
	44: "flammable-material.storage.detected",
	45: "fire-lane.obstruction.detected",
	46: "sleep-on-duty.detected",
	47: "fire-extinguisher.location.detected",
	48: "motor-vehicle.reverse-driving.detected",
	49: "vehicle-traffic.statistic",
	50: "non-vehicle.wrong-way-driving.detected",
	51: "water-gauge.level.measured",
	52: "large-construction-vehicle.inspected",
	53: "floating-object.detected",
	54: "hazardous-materials-vehicle.nighttime.detected",
	56: "work-uniform.recognition",
	57: "unenclosed-vehicle-cargo.detected",
	58: "face.recognized",
	59: "license-plate.recognized",
	60: "failure-to-yield.detected",
	61: "solid-line-u-turn.detected",
	62: "emergency-vehicle.captured",
	63: "electric-vehicle.red-light-violation.captured",
	64: "large-bus.nighttime-ban.detected",
	65: "fighting.detected",
	67: "person-climbing.detected",
	68: "unlicensed-street-vendors.detected",
	69: "large-model.detected",
	70: "third-party-algorithm.detected",
}

// humanAttributeCodes maps each eventAttribute field name to its integer → label table.
// Source: AIBOX Human Attribute API spec.
var humanAttributeCodes = map[string]map[int]string{
	"age": {
		0: "Unknown", 1: "Child", 2: "Young Adult", 3: "Middle-Aged Adult", 4: "Elderly Adult",
	},
	"gender": {
		0: "Unknown", 1: "Male", 2: "Female",
	},
	"upper": {
		0: "Unknown", 1: "Short Sleeves", 2: "Long Sleeves",
	},
	"upperColor": {
		0: "Unknown", 1: "Black", 2: "Brown", 3: "Blue", 4: "Green",
		5: "Gray", 6: "Orange", 7: "Pink", 8: "Purple", 9: "Red", 10: "White", 11: "Yellow",
	},
	"upperTexture": {
		0: "Unknown", 1: "Plaid", 2: "Floral Print", 3: "Solid Color", 4: "Stripes",
	},
	"lower": {
		0: "Unknown", 1: "Shorts", 2: "Long Pants",
	},
	"lowerColor": {
		0: "Unknown", 1: "Black", 2: "Brown", 3: "Blue", 4: "Green",
		5: "Gray", 6: "Orange", 7: "Pink", 8: "Purple", 9: "Red", 10: "White", 11: "Yellow",
	},
	"skirt": {
		0: "Unknown", 1: "Wearing Skirt", 2: "Not Wearing Skirt",
	},
	"hat": {
		0: "Unknown", 1: "Wearing Hat", 2: "Not Wearing Hat",
	},
	"mask": {
		0: "Unknown", 1: "Wearing Mask", 2: "Not Wearing Mask",
	},
	"backPack": {
		0: "Unknown", 1: "Backpack", 2: "Shoulder Bag", 3: "Handbag", 4: "No Bag",
	},
	"riding": {
		0: "Unknown", 1: "Riding", 2: "Not Riding",
	},
	"direction": {
		0: "Unknown", 1: "Front", 2: "Side", 3: "Back",
	},
	"hair": {
		0: "Unknown", 1: "Short Hair", 2: "Long Hair", 3: "Updo", 4: "Bald", 5: "Medium-Length Hair",
	},
	"shoe": {
		0: "Unknown", 1: "Leather Shoes", 2: "Sandals", 3: "Casual Shoes", 4: "Knee-High Boots",
	},
	"shoeColor": {
		0: "Unknown", 1: "Black", 2: "Brown", 3: "Blue", 4: "Green",
		5: "Gray", 6: "Orange", 7: "Pink", 8: "Purple", 9: "Red", 10: "White", 11: "Yellow",
	},
}

// TranslateEventAttribute returns a copy of attrs with integer-coded fields replaced
// by their human-readable labels. Unknown codes or non-integer values pass through unchanged.
// Non-attribute fields (e.g. string fields like vestName, featureImageId) are kept as-is.
func TranslateEventAttribute(attrs map[string]any) map[string]any {
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		codes, hasCodes := humanAttributeCodes[k]
		if !hasCodes {
			out[k] = v
			continue
		}
		n := toInt(v)
		if n < 0 {
			out[k] = v
			continue
		}
		if label, ok := codes[n]; ok {
			out[k] = label
		} else {
			out[k] = v
		}
	}
	return out
}

// ResolveEventType returns the canonical eventType for an AIBOX alarmType code.
// Falls back to fallback (usually canonical.EventType) when code is not in the table.
func ResolveEventType(alarmType int, fallback string) string {
	if et, ok := AlarmTypeEventType[alarmType]; ok {
		return et
	}
	return fallback
}

// AlarmTypeFromPayload extracts the alarm type integer from an AIBOX event payload.
// AIBOX sends the alarm code as "typeValue"; "alarmType" is accepted as a fallback
// for older firmware and template-mapped payloads.
// Returns -1 when neither field is present or parseable.
func AlarmTypeFromPayload(payload map[string]any) int {
	if payload == nil {
		return -1
	}
	for _, key := range []string{"typeValue", "alarmType"} {
		v, ok := payload[key]
		if !ok {
			continue
		}
		n := toInt(v)
		if n >= 0 {
			return n
		}
	}
	return -1
}

// toInt converts numeric JSON types to int. Returns -1 when not parseable.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	}
	return -1
}
