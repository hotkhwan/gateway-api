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
// later crop drops instead if a frame is unusually large.
const maxDahuaPicBytes = 700 * 1024

// dahuaPictureBase64List extracts the embedded JPEG snapshots from a Dahua
// multipart event and returns them base64-encoded for the event payload's
// "pictureBase64List" (the same field AIBOX uses), so the existing normalizer
// S3 path (extractBinaries) uploads them and emits binaryRefs.
//
// Two images are produced — the full scene (pictureList_0) and the detected
// subject crop (pictureList_1): VehicleBody for a vehicle, HumanImage for a
// person, FaceImage for a face. Each image's byte range is described in the
// metadata; we reassemble the binary blob, slice each range, and keep only
// well-formed JPEGs — so a wrong-offset slice degrades to "no image", never
// garbage. Returns nil when nothing valid is found.
func dahuaPictureBase64List(contentType string, body []byte, rawBody map[string]any) []any {
	blob := concatMultipartBinary(contentType, body)
	if len(blob) == 0 {
		return nil
	}
	data := dahuaEventData(rawBody)
	if data == nil {
		return nil
	}
	return encodeImageDescs(blob, dahuaImageDescriptors(data))
}

// dahuaImageDescriptors returns up to two image descriptors — full scene then
// subject body crop — handling the several shapes Dahua firmwares emit:
//   - Type-tagged Image[] array   (SceneImage + VehicleBody / HumanImage / …)
//   - direct keys                 (SceneImage + HumanImage / FaceImage / VehicleBodyImage / …)
//   - nested legacy keys          (SceneImage + Vehicle.Image / Object.Image)
func dahuaImageDescriptors(data map[string]any) []map[string]any {
	// 1) Type-tagged Image[] array.
	if arr, ok := data["Image"].([]any); ok && len(arr) > 0 {
		return selectFromImageArray(arr)
	}

	// 2) Direct keys (HumanTrait / FaceTrait / TrafficJunction firmwares), then
	//    nested legacy keys. HumanImage is preferred over FaceImage so a
	//    pedestrian event shows the person, not just the face.
	scene := imageMap(data["SceneImage"])
	if scene == nil {
		scene = imageMap(data["FaceSceneImage"])
	}
	crop := firstImageMap(data, "HumanImage", "FaceImage", "VehicleBodyImage", "VehicleImage", "NonMotorImage")
	if crop == nil {
		crop = firstNestedImage(data, "Vehicle", "Object", "Human", "NonMotor")
	}

	var out []map[string]any
	if scene != nil {
		out = append(out, scene)
	}
	if crop != nil {
		out = append(out, crop)
	}
	return out
}

// selectFromImageArray picks [SceneImage, bodyCrop] from a Type-tagged Image[]
// array. The scene goes first; the crop is the Type matching the detected subject
// (VehicleBody / HumanImage / FaceImage / NonMotor*), falling back to the first
// non-scene image.
func selectFromImageArray(arr []any) []map[string]any {
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
		out = append(out, scene)
	}
	if body != nil {
		out = append(out, body)
	}
	return out
}

// isBodyImageType reports whether a Dahua Image.Type is the detected-subject crop.
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

// imageMap returns v as an image descriptor map (carrying Offset/Length), or nil
// — so non-image objects (e.g. HumanAttributes) under the same Data are ignored.
func imageMap(v any) map[string]any {
	m := asMap(v)
	if m == nil {
		return nil
	}
	_, hasOff := m["Offset"]
	_, hasLen := m["Length"]
	if !hasOff && !hasLen {
		return nil
	}
	return m
}

// firstImageMap returns the first of keys whose value is an image descriptor.
func firstImageMap(data map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		if m := imageMap(data[k]); m != nil {
			return m
		}
	}
	return nil
}

// firstNestedImage returns the first groups[i].Image that is an image descriptor.
func firstNestedImage(data map[string]any, groups ...string) map[string]any {
	for _, g := range groups {
		if m := imageMap(asMap(data[g])["Image"]); m != nil {
			return m
		}
	}
	return nil
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
