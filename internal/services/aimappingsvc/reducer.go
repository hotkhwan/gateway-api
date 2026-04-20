// internal/services/aimappingsvc/reducer.go
package aimappingsvc

import (
	"encoding/json"
	"strings"
)

const (
	maxArrayElements = 2
	maxObjectDepth   = 5
	maxStringLength  = 256
	hardCapBytes     = 16 * 1024 // 16 KB hard cap

	// noiseFields are known high-noise fields to strip before sending to AI.
	noiseFieldImageBase64 = "imageBase64"
	noiseFieldRawFrame    = "rawFrame"
	noiseFieldBinaryData  = "binaryData"
	noiseFieldDebugDump   = "debugDump"
)

var noiseFieldSet = map[string]bool{
	noiseFieldImageBase64: true,
	noiseFieldRawFrame:    true,
	noiseFieldBinaryData:  true,
	noiseFieldDebugDump:   true,
}

// ReduceResult is the output of ReducePayload.
type ReduceResult struct {
	ObservedPaths    []string
	ReducedPayload   map[string]any
	TruncatedArrays  int
	TruncatedStrings int
	DroppedPaths     []string
	ReducedBytes     int
}

// ReducePayload reduces raw payload for AI prompt:
//  1. Remove known noise fields (imageBase64, rawFrame, binaryData, debugDump)
//  2. Sanitize BSON-unsafe key chars ($ → _, . → _)
//  3. Bound array length → 2 elements
//  4. Bound object depth → 5 layers
//  5. Bound string length → 256 chars
//  6. Build observedPaths (all field paths, even truncated)
func ReducePayload(raw map[string]any, maxBytes int) ReduceResult {
	if maxBytes <= 0 || maxBytes > hardCapBytes {
		maxBytes = hardCapBytes
	}

	res := &ReduceResult{}
	reduced := reduceObject(raw, "", 0, res)
	res.ReducedPayload = reduced

	// Compute byte size of reduced payload.
	if encoded, err := json.Marshal(reduced); err == nil {
		size := len(encoded)
		if size > maxBytes {
			// Trim payload to fit within maxBytes by marshaling with limit.
			// We do a simple approach: marshal and truncate the map keys until it fits.
			res.ReducedPayload = trimToBytes(reduced, maxBytes)
			if reEncoded, err2 := json.Marshal(res.ReducedPayload); err2 == nil {
				res.ReducedBytes = len(reEncoded)
			} else {
				res.ReducedBytes = maxBytes
			}
		} else {
			res.ReducedBytes = size
		}
	}

	return *res
}

// reduceObject recursively traverses and reduces a map.
func reduceObject(obj map[string]any, prefix string, depth int, res *ReduceResult) map[string]any {
	if depth >= maxObjectDepth {
		return map[string]any{}
	}
	out := make(map[string]any, len(obj))
	for rawKey, val := range obj {
		// Strip noise fields at any depth by raw key name.
		if noiseFieldSet[rawKey] {
			path := buildPath(prefix, rawKey)
			res.DroppedPaths = append(res.DroppedPaths, path)
			continue
		}

		// Sanitize BSON-unsafe key chars.
		key := sanitizeKey(rawKey)
		path := buildPath(prefix, key)
		// Record observed path.
		res.ObservedPaths = append(res.ObservedPaths, path)

		out[key] = reduceValue(val, path, depth+1, res)
	}
	return out
}

// reduceValue recursively reduces a value.
func reduceValue(val any, path string, depth int, res *ReduceResult) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		if len(v) > maxStringLength {
			res.TruncatedStrings++
			return v[:maxStringLength]
		}
		return v
	case map[string]any:
		if depth >= maxObjectDepth {
			return map[string]any{}
		}
		return reduceObject(v, path, depth, res)
	case []any:
		return reduceArray(v, path, depth, res)
	default:
		// Numbers, booleans, etc. pass through as-is.
		return v
	}
}

// reduceArray truncates arrays to maxArrayElements.
func reduceArray(arr []any, path string, depth int, res *ReduceResult) []any {
	if len(arr) > maxArrayElements {
		res.TruncatedArrays++
		arr = arr[:maxArrayElements]
	}
	out := make([]any, 0, len(arr))
	for _, item := range arr {
		out = append(out, reduceValue(item, path+"[]", depth, res))
	}
	return out
}

// sanitizeKey replaces BSON-unsafe characters in field keys.
func sanitizeKey(key string) string {
	key = strings.ReplaceAll(key, "$", "_")
	key = strings.ReplaceAll(key, ".", "_")
	return key
}

// buildPath constructs a dotted field path.
func buildPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// trimToBytes removes top-level keys until the marshaled size fits within maxBytes.
// This is a best-effort trim — the result may still slightly exceed maxBytes.
func trimToBytes(obj map[string]any, maxBytes int) map[string]any {
	for {
		encoded, err := json.Marshal(obj)
		if err != nil || len(encoded) <= maxBytes {
			return obj
		}
		// Remove one arbitrary key to shrink the payload.
		removed := false
		for k := range obj {
			delete(obj, k)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	return obj
}
