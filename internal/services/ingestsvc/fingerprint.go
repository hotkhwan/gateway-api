// internal/services/ingestsvc/fingerprint.go
package ingestsvc

import (
	"fmt"
	"sort"
	"strings"
)

// BuildFingerprint builds a deterministic fingerprint from a JSON payload map.
//
// Format: "key1:type|key2:type|..." (sorted, flattened, value-type normalized)
//
// Rules:
//   - keys are flattened using dot notation (e.g. "location.lat")
//   - values are normalized to type string: string, number, boolean, null, object, array
//   - keys are sorted alphabetically for stability
//   - values are NOT included — only key+type pairs
//
// Example payload:
//
//	{"cameraId": "cam-001", "location": {"lat": 13.7, "lng": 100.5}}
//
// Result:
//
//	"cameraId:string|location.lat:number|location.lng:number"
func BuildFingerprint(payload map[string]any) string {
	pairs := flattenTypePairs("", payload)
	sort.Strings(pairs)
	return strings.Join(pairs, "|")
}

// flattenTypePairs recursively flattens a map into "path:type" pairs.
func flattenTypePairs(prefix string, obj map[string]any) []string {
	var pairs []string
	for k, v := range obj {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			// recurse into nested object
			pairs = append(pairs, flattenTypePairs(path, val)...)
		case []any:
			_ = val
			pairs = append(pairs, fmt.Sprintf("%s:array", path))
		case nil:
			pairs = append(pairs, fmt.Sprintf("%s:null", path))
		case bool:
			_ = val
			pairs = append(pairs, fmt.Sprintf("%s:boolean", path))
		case float64, int, int64, float32:
			pairs = append(pairs, fmt.Sprintf("%s:number", path))
		case string:
			_ = val
			pairs = append(pairs, fmt.Sprintf("%s:string", path))
		default:
			pairs = append(pairs, fmt.Sprintf("%s:string", path))
		}
	}
	return pairs
}
