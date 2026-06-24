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
// dropped; the full scene is added first so it (pictureList_0) is kept and the
// later crops drop instead if a frame is unusually large.
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
// Order: full scene (pictureList_0) → the event's body crop (pictureList_1), so
// the consumer convention "pictureList_0 = full, pictureList_1 = crop" holds —
// VehicleBody for a vehicle event, HumanImage for a person event, etc.
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

	// Preferred shape: Events[0].Data.Image is a Type-tagged array
	// (SceneImage + VehicleBody / HumanImage / …) — pick scene + the body crop.
	if descs := selectDahuaImages(data); len(descs) > 0 {
		return encodeImageDescs(blob, descs)
	}

	// Fallback (legacy firmware): nested SceneImage / Vehicle.Image / Object.Image keys.
	return encodeImageDescs(blob, []map[string]any{
		imageDesc(data, "SceneImage"),
		imageDesc(asMap(data["Vehicle"]), "Image"),
		imageDesc(asMap(data["Object"]), "Image"),
	})
}

// selectDahuaImages picks [SceneImage, bodyCrop] from the Events[0].Data.Image
// array. The scene goes first (full frame); the crop is the Type matching the
// detected subject (VehicleBody / HumanImage / FaceImage / NonMotor*), falling
// back to the first non-scene image. Returns nil when the array is absent.
func selectDahuaImages(data map[string]any) []map[string]any {
	arr, ok := data["Image"].([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	var scene, body, firstOther map[string]any
	for _, it := range arr {
		m := asMap(it)
		if m == nil {
			continue
		}
		switch t := asStr(m["Type"]); {
		case t == "SceneImage":
			if scene == nil {
				scene = m
			}
		case isBodyImageType(t):
			if body == nil {
				body = m
			}
		default:
			if firstOther == nil {
				firstOther = m
			}
		}
	}
	if body == nil {
		body = firstOther
	}
	var out []map[string]any
	if scene != nil {
		out = append(out, scene) // pictureList_0 = full scene
	}
	if body != nil {
		out = append(out, body) // pictureList_1 = body crop
	}
	return out
}

// isBodyImageType reports whether a Dahua Image.Type is the detected-subject crop
// (as opposed to the scene or a sub-detail like a plate).
func isBodyImageType(t string) bool {
	switch t {
	case "VehicleBody", "HumanImage", "HumanBody", "FaceImage", "NonMotorBody", "NonMotorImage", "NonMotor":
		return true
	}
	return false
}

// encodeImageDescs slices each {Offset,Length} descriptor out of the binary blob,
// validates it is a JPEG, and returns the base64 list. The cumulative size cap
// keeps the raw.events message under the Kafka limit (earlier descriptors win).
func encodeImageDescs(blob []byte, descs []map[string]any) []any {
	var out []any
	used := 0
	for _, img := range descs {
		if img == nil {
			continue
		}
		off := looseInt(img["Offset"])
		length := looseInt(img["Length"])
		if length <= 0 || off < 0 || off+length > len(blob) {
			continue
		}
		if used+length > maxDahuaPicBytes {
			continue
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

func asStr(v any) string {
	s, _ := v.(string)
	return s
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
