// internal/services/ingestsvc/dahuaImages.go
package ingestsvc

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
)

// maxDahuaPicBytes caps the cumulative RAW snapshot bytes carried as base64 in
// one event, so the raw.events Kafka message stays well under the broker max
// (base64 ≈ 4/3 × raw; 700 KB raw ≈ 0.93 MB base64). Images past the cap are
// dropped (vehicle + plate are added before the larger scene frame).
const maxDahuaPicBytes = 700 * 1024

// dahuaPictureBase64List extracts the embedded JPEG snapshots from a Dahua
// multipart event and returns them base64-encoded, ready to drop into the event
// payload under "pictureBase64List" (the same field AIBOX uses) so the existing
// normalizer S3 path (extractBinaries) uploads them and emits binaryRefs.
//
// Dahua concatenates the snapshots into the multipart's binary part(s); each
// image's byte range is described in the metadata under
// Events[0].Data.{Vehicle.Image, Object.Image, SceneImage}.{Offset,Length}.
// We reassemble the binary blob, slice each range, and keep only well-formed
// JPEGs — so a wrong-offset slice degrades to "no image", never garbage.
//
// Order: vehicle (subject) → object/plate (detail) → scene (context).
// Returns nil when nothing valid is found.
func dahuaPictureBase64List(contentType string, body []byte, rawBody map[string]any) []any {
	blob := concatMultipartBinary(contentType, body)
	if len(blob) == 0 {
		return nil
	}
	data := dahuaEventData(rawBody)
	if data == nil {
		return nil
	}

	descriptors := []map[string]any{
		imageDesc(asMap(data["Vehicle"]), "Image"), // vehicle crop
		imageDesc(asMap(data["Object"]), "Image"),  // plate / object crop
		imageDesc(data, "SceneImage"),              // full scene
	}

	var out []any
	used := 0
	for _, img := range descriptors {
		if img == nil {
			continue
		}
		off := looseInt(img["Offset"])
		length := looseInt(img["Length"])
		if length <= 0 || off < 0 || off+length > len(blob) {
			continue
		}
		if used+length > maxDahuaPicBytes {
			continue // skip this one (likely the large scene), keep smaller ones
		}
		chunk := blob[off : off+length]
		if !looksJPEG(chunk) {
			continue
		}
		out = append(out, base64.StdEncoding.EncodeToString(chunk))
		used += length
	}
	return out
}

// concatMultipartBinary reads every binary part of a multipart body IN FULL
// (no size cap) and concatenates them — reconstructing the image data blob that
// Dahua's Offset/Length descriptors index into. Text/metadata parts are skipped.
func concatMultipartBinary(contentType string, body []byte) []byte {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var blob []byte
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		pct := part.Header.Get("Content-Type")
		data, _ := io.ReadAll(part)
		_ = part.Close()
		if isBinaryPart(pct) {
			blob = append(blob, data...)
		}
	}
	return blob
}

// dahuaEventData returns Events[0].Data from a decoded Dahua metadata body.
func dahuaEventData(rawBody map[string]any) map[string]any {
	if rawBody == nil {
		return nil
	}
	events, ok := rawBody["Events"].([]any)
	if !ok || len(events) == 0 {
		return nil
	}
	ev, ok := events[0].(map[string]any)
	if !ok {
		return nil
	}
	return asMap(ev["Data"])
}

// imageDesc returns obj[key] as a map (an {Offset,Length,...} image descriptor).
func imageDesc(obj map[string]any, key string) map[string]any {
	if obj == nil {
		return nil
	}
	return asMap(obj[key])
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// looseInt coerces a JSON-decoded number (float64) or int to int; -1 otherwise.
func looseInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return -1
}

// looksJPEG reports whether b starts with the JPEG SOI marker (FF D8).
func looksJPEG(b []byte) bool {
	return len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8
}
