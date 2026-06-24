// internal/services/ingestsvc/dahuaImages_test.go
package ingestsvc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"testing"
)

// jpeg builds an n-byte fake JPEG (SOI marker + tag, zero-padded).
func jpeg(tag string, n int) []byte {
	b := append([]byte{0xFF, 0xD8}, []byte(tag)...)
	for len(b) < n {
		b = append(b, 0)
	}
	return b[:n]
}

// dahuaMultipart wraps a metadata JSON + a concatenated binary blob into a
// Dahua-style multipart/x-mixed-replace body.
func dahuaMultipart(t *testing.T, meta string, blob []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	th := textproto.MIMEHeader{}
	th.Set("Content-Type", "text/plain")
	pw, _ := w.CreatePart(th)
	_, _ = pw.Write([]byte(meta))
	bh := textproto.MIMEHeader{}
	bh.Set("Content-Type", "image/jpeg")
	bw, _ := w.CreatePart(bh)
	_, _ = bw.Write(blob)
	_ = w.Close()
	return "multipart/x-mixed-replace; boundary=" + w.Boundary(), buf.Bytes()
}

func decodePics(t *testing.T, pics []any) [][]byte {
	t.Helper()
	out := make([][]byte, 0, len(pics))
	for i, p := range pics {
		s, ok := p.(string)
		if !ok {
			t.Fatalf("pic %d not a string", i)
		}
		dec, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("pic %d not base64: %v", i, err)
		}
		out = append(out, dec)
	}
	return out
}

func run(t *testing.T, meta string, blob []byte) [][]byte {
	t.Helper()
	ct, body := dahuaMultipart(t, meta, blob)
	var raw map[string]any
	if err := json.Unmarshal([]byte(meta), &raw); err != nil {
		t.Fatal(err)
	}
	return decodePics(t, dahuaPictureBase64List(ct, body, raw))
}

// --- HumanTrait: direct keys SceneImage + HumanImage (+ FaceImage) -----------
func TestDahuaImages_humanDirectKeys(t *testing.T) {
	scn, hum, fac := jpeg("SCN", 300), jpeg("HUM", 120), jpeg("FAC", 40)
	blob := bytes.Join([][]byte{scn, hum, fac}, nil)
	// HumanImage is preferred over FaceImage for a person event.
	meta := fmt.Sprintf(`{"Events":[{"Code":"HumanTrait","Data":{`+
		`"SceneImage":{"Offset":0,"Length":%d,"Width":1920,"Height":1080},`+
		`"HumanImage":{"Offset":%d,"Length":%d},`+
		`"FaceImage":{"Offset":%d,"Length":%d},`+
		`"HumanAttributes":{"age":3}}}]}`,
		len(scn), len(scn), len(hum), len(scn)+len(hum), len(fac))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], hum) {
		t.Fatalf("want [scene, human], got %d images", len(got))
	}
}

// --- Vehicle: nested legacy SceneImage + Vehicle.Image -----------------------
func TestDahuaImages_vehicleNestedLegacy(t *testing.T) {
	scn, veh := jpeg("SCN", 200), jpeg("VEH", 130)
	blob := bytes.Join([][]byte{scn, veh}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Code":"TrafficJunction","Data":{`+
		`"SceneImage":{"Offset":0,"Length":%d},`+
		`"Vehicle":{"Image":{"Offset":%d,"Length":%d}},`+
		`"Object":{"Image":{"Offset":0,"Length":0}}}}]}`,
		len(scn), len(scn), len(veh))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Fatalf("want [scene, vehicle], got %d images", len(got))
	}
}

// --- Type-tagged Image[] array ----------------------------------------------
func TestDahuaImages_imageArrayVehicle(t *testing.T) {
	scn, veh := jpeg("SCN", 200), jpeg("VEH", 130)
	blob := bytes.Join([][]byte{scn, veh}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Data":{"Image":[`+
		`{"Type":"SceneImage","Offset":0,"Length":%d},`+
		`{"Type":"VehicleBody","Offset":%d,"Length":%d}]}}]}`,
		len(scn), len(scn), len(veh))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Fatalf("want [scene, vehicleBody], got %d images", len(got))
	}
}

func TestDahuaImages_imageArrayHuman(t *testing.T) {
	scn, hum := jpeg("SCN", 220), jpeg("HUM", 105)
	blob := bytes.Join([][]byte{scn, hum}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Data":{"Image":[`+
		`{"Type":"SceneImage","Offset":0,"Length":%d},`+
		`{"Type":"HumanImage","Offset":%d,"Length":%d}]}}]}`,
		len(scn), len(scn), len(hum))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], hum) {
		t.Fatalf("want [scene, humanImage], got %d images", len(got))
	}
}

// --- size cap drops the crop when the scene alone is huge -------------------
func TestDahuaImages_sizeCap(t *testing.T) {
	scn := jpeg("SCN", maxDahuaPicBytes-50)
	hum := jpeg("HUM", 200) // would push over the cap → dropped
	blob := bytes.Join([][]byte{scn, hum}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Data":{`+
		`"SceneImage":{"Offset":0,"Length":%d},`+
		`"HumanImage":{"Offset":%d,"Length":%d}}}]}`,
		len(scn), len(scn), len(hum))
	got := run(t, meta, blob)
	if len(got) != 1 || !bytes.Equal(got[0], scn) {
		t.Fatalf("want [scene] only (crop over cap), got %d images", len(got))
	}
}

// Vehicle: the car-body crop must win over the license-plate crop, even when the
// plate appears first / in a different shape than the body.
func TestDahuaImages_vehicleBodyBeatsPlate(t *testing.T) {
	scn, plate, veh := jpeg("SCN", 200), jpeg("PLT", 30), jpeg("VEH", 130)
	blob := bytes.Join([][]byte{scn, plate, veh}, nil)
	// Plate is in the Image[] array (Type "Plate"); the body is the nested
	// Vehicle.Image — the code must still prefer the body.
	meta := fmt.Sprintf(`{"Events":[{"Code":"TrafficJunction","Data":{`+
		`"Image":[{"Type":"SceneImage","Offset":0,"Length":%d},`+
		`{"Type":"Plate","Offset":%d,"Length":%d}],`+
		`"Object":{"ObjectType":"Plate","Image":{"Offset":%d,"Length":%d}},`+
		`"Vehicle":{"Image":{"Offset":%d,"Length":%d}}}}]}`,
		len(scn), len(scn), len(plate), len(scn), len(plate), len(scn)+len(plate), len(veh))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Fatalf("want [scene, vehicleBody], got %d images (plate must not win)", len(got))
	}
}

// When only a plate crop exists (no body), it is used as the last resort.
func TestDahuaImages_plateLastResort(t *testing.T) {
	scn, plate := jpeg("SCN", 200), jpeg("PLT", 30)
	blob := bytes.Join([][]byte{scn, plate}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Data":{`+
		`"SceneImage":{"Offset":0,"Length":%d},`+
		`"Object":{"ObjectType":"Plate","Image":{"Offset":%d,"Length":%d}}}}]}`,
		len(scn), len(scn), len(plate))
	got := run(t, meta, blob)
	if len(got) != 2 || !bytes.Equal(got[1], plate) {
		t.Fatalf("want [scene, plate] as last resort, got %d images", len(got))
	}
}

func TestDahuaImages_noEventsReturnsNil(t *testing.T) {
	ct, body := dahuaMultipart(t, `{"Ack":true}`, jpeg("X", 50))
	var raw map[string]any
	_ = json.Unmarshal([]byte(`{"Ack":true}`), &raw)
	if pics := dahuaPictureBase64List(ct, body, raw); pics != nil {
		t.Fatalf("want nil, got %d pics", len(pics))
	}
}

func TestDahuaImages_skipsNonJPEG(t *testing.T) {
	scn := []byte("NOT-A-JPEG-NO-SOI-MARKER!!") // no FFD8
	hum := jpeg("HUM", 80)
	blob := bytes.Join([][]byte{scn, hum}, nil)
	meta := fmt.Sprintf(`{"Events":[{"Data":{`+
		`"SceneImage":{"Offset":0,"Length":%d},`+
		`"HumanImage":{"Offset":%d,"Length":%d}}}]}`,
		len(scn), len(scn), len(hum))
	got := run(t, meta, blob)
	if len(got) != 1 || !bytes.Equal(got[0], hum) {
		t.Fatalf("want [human] only (scene not JPEG), got %d images", len(got))
	}
}
