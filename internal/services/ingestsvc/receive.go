// internal/services/ingestsvc/receive.go
package ingestsvc

import (
	"encoding/json"
	"strings"
)

// decodeRawBody decodes raw bytes into map[string]any and sanitizes BSON-unsafe keys.
// If JSON parsing fails, stores the raw string under "_raw" key.
func decodeRawBody(data []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return map[string]any{"_raw": string(data)}, nil
	}
	return sanitizeBSONKeys(body), nil
}

// sanitizeBSONKeys replaces "." and "$" in map keys recursively.
// BSON does not allow these characters in field names.
func sanitizeBSONKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		safe := strings.ReplaceAll(k, ".", "_")
		safe = strings.ReplaceAll(safe, "$", "_")
		if nested, ok := v.(map[string]any); ok {
			v = sanitizeBSONKeys(nested)
		}
		out[safe] = v
	}
	return out
}
