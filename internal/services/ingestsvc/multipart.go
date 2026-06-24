// internal/services/ingestsvc/multipart.go
package ingestsvc

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// maxMultipartTextPart caps how much of a single text part we keep, so a
// malformed/huge text part can't reinflate the event we just slimmed down.
const maxMultipartTextPart = 64 * 1024

// isMultipartContentType reports whether ct is any multipart/* media type
// (Dahua/Hikvision cameras push multipart/x-mixed-replace event streams).
func isMultipartContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		// ParseMediaType is strict; fall back to a cheap prefix check.
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "multipart/")
	}
	return strings.HasPrefix(mt, "multipart/")
}

// normalizeMultipart turns a multipart body (e.g. Dahua multipart/x-mixed-replace,
// metadata text part + JPEG snapshot parts) into a single JSON object:
//
//   - text / JSON parts are merged into the payload (so the real event fields
//     surface for template matching instead of a 414 KB opaque blob);
//   - binary parts (image/*, octet-stream) are reduced to lightweight
//     descriptors under "_binaries" — the snapshot bytes are NOT inlined, which
//     keeps the canonical event small and Kafka-safe. (Uploading snapshots to
//     S3 is a later phase.)
//
// Returns the JSON bytes and true on success, or false when the body is not
// parseable multipart (caller then keeps the original body).
func normalizeMultipart(contentType string, body []byte) ([]byte, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, false
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, false
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	payload := map[string]any{}
	var binaries []map[string]any
	idx := -1
	sawPart := false

	for {
		part, err := mr.NextPart()
		if err != nil {
			break // io.EOF or a malformed tail — stop with what we have
		}
		idx++
		sawPart = true

		pct := part.Header.Get("Content-Type")
		data, _ := io.ReadAll(io.LimitReader(part, maxMultipartTextPart+1))
		_ = part.Close()

		if isBinaryPart(pct) {
			binaries = append(binaries, map[string]any{
				"index":       idx,
				"contentType": pct,
				"size":        len(data),
			})
			continue
		}
		mergeTextPart(payload, data)
	}

	if !sawPart {
		return nil, false
	}
	if len(binaries) > 0 {
		payload["_binaries"] = binaries
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return out, true
}

// isBinaryPart reports whether a part's Content-Type is a non-text payload
// (snapshot image / raw bytes) that must not be inlined into the event.
func isBinaryPart(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return false // untyped → treat as text/metadata
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mt = ct
	}
	switch {
	case strings.HasPrefix(mt, "image/"),
		strings.HasPrefix(mt, "video/"),
		strings.HasPrefix(mt, "audio/"),
		strings.HasPrefix(mt, "application/octet-stream"):
		return true
	default:
		return false
	}
}

// mergeTextPart extracts structured fields from one text part into payload.
// Handles three shapes, in order:
//   - a whole-part JSON object;
//   - Dahua "Key=Value;...;data={json}" — leading key/values plus the embedded
//     JSON under "data";
//   - plain "key=value;..." pairs.
//
// Anything that yields no structure is kept verbatim under "_text" so no data
// is silently dropped before a template exists.
func mergeTextPart(payload map[string]any, raw []byte) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return
	}
	if len(s) > maxMultipartTextPart {
		s = s[:maxMultipartTextPart]
	}

	// (a) whole part is a JSON object.
	if obj, ok := asJSONObject(s); ok {
		for k, v := range obj {
			payload[k] = v
		}
		return
	}

	// (b) Dahua "Code=..;action=..;data={json}" form.
	if i := strings.Index(s, "data="); i >= 0 {
		parseKVPairs(payload, s[:i])
		rest := s[i+len("data="):]
		if l := strings.Index(rest, "{"); l >= 0 {
			if r := strings.LastIndex(rest, "}"); r > l {
				if obj, ok := asJSONObject(strings.TrimSpace(rest[l : r+1])); ok {
					payload["data"] = obj
					return
				}
			}
		}
	}

	// (c) plain key=value;... pairs.
	if strings.Contains(s, "=") {
		before := len(payload)
		parseKVPairs(payload, s)
		if len(payload) > before {
			return
		}
	}

	// (d) unstructured — keep verbatim.
	if existing, ok := payload["_text"].([]any); ok {
		payload["_text"] = append(existing, s)
	} else {
		payload["_text"] = []any{s}
	}
}

func asJSONObject(s string) (map[string]any, bool) {
	if !strings.HasPrefix(s, "{") {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, false
	}
	return m, true
}

func parseKVPairs(payload map[string]any, s string) {
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		if k == "" {
			continue
		}
		payload[k] = strings.TrimSpace(kv[1])
	}
}
