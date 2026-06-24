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

func metaFor(lenV, lenO, lenS int) string {
	return fmt.Sprintf(`{"Events":[{"Code":"TrafficJunction","Data":{`+
		`"Vehicle":{"Image":{"Offset":0,"Length":%d}},`+
		`"Object":{"Image":{"Offset":%d,"Length":%d}},`+
		`"SceneImage":{"Offset":%d,"Length":%d}}}]}`,
		lenV, lenV, lenO, lenV+lenO, lenS)
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

func TestDahuaPictureBase64List_extractsInOrder(t *testing.T) {
	veh, obj, scn := jpeg("VEH", 120), jpeg("OBJ", 40), jpeg("SCN", 300)
	blob := bytes.Join([][]byte{veh, obj, scn}, nil)
	ct, body := dahuaMultipart(t, metaFor(len(veh), len(obj), len(scn)), blob)

	var raw map[string]any
	if err := json.Unmarshal([]byte(metaFor(len(veh), len(obj), len(scn))), &raw); err != nil {
		t.Fatal(err)
	}
	pics := dahuaPictureBase64List(ct, body, raw)
	got := decodePics(t, pics)
	want := [][]byte{scn, veh, obj} // scene (full) → vehicle → object
	if len(got) != 3 {
		t.Fatalf("want 3 pics, got %d", len(got))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("pic %d mismatch", i)
		}
	}
}

func TestDahuaPictureBase64List_skipsNonJPEGandOutOfBounds(t *testing.T) {
	veh := jpeg("VEH", 80)
	obj := []byte("NOTJPEGDATA-NOFFD8-HEADER!!") // no SOI → skipped
	scn := jpeg("SCN", 100)
	blob := bytes.Join([][]byte{veh, obj, scn}, nil)
	// Object length deliberately overruns the blob → out-of-bounds, skipped.
	meta := fmt.Sprintf(`{"Events":[{"Code":"x","Data":{`+
		`"Vehicle":{"Image":{"Offset":0,"Length":%d}},`+
		`"Object":{"Image":{"Offset":%d,"Length":99999}},`+
		`"SceneImage":{"Offset":%d,"Length":%d}}}]}`,
		len(veh), len(veh), len(veh)+len(obj), len(scn))
	ct, body := dahuaMultipart(t, meta, blob)
	var raw map[string]any
	_ = json.Unmarshal([]byte(meta), &raw)

	pics := dahuaPictureBase64List(ct, body, raw)
	got := decodePics(t, pics)
	if len(got) != 2 { // scene + vehicle; object skipped (OOB)
		t.Fatalf("want 2 pics, got %d", len(got))
	}
	if !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Errorf("expected scene (full) then vehicle")
	}
}

func TestDahuaPictureBase64List_sizeCapDropsLargeScene(t *testing.T) {
	veh, obj := jpeg("VEH", 100), jpeg("OBJ", 50)
	scn := jpeg("SCN", maxDahuaPicBytes+10) // over the cap → dropped
	blob := bytes.Join([][]byte{veh, obj, scn}, nil)
	ct, body := dahuaMultipart(t, metaFor(len(veh), len(obj), len(scn)), blob)
	var raw map[string]any
	_ = json.Unmarshal([]byte(metaFor(len(veh), len(obj), len(scn))), &raw)

	pics := dahuaPictureBase64List(ct, body, raw)
	if len(pics) != 2 { // vehicle + object kept, scene dropped
		t.Fatalf("want 2 pics (scene dropped), got %d", len(pics))
	}
}

// metaImageArray builds an Events[0].Data.Image array (Type-tagged) for a
// scene + body crop laid out contiguously in the blob.
func metaImageArray(bodyType string, lenScene, lenBody int) string {
	return fmt.Sprintf(`{"Events":[{"Data":{"Image":[`+
		`{"Type":"SceneImage","Offset":0,"Length":%d,"Width":1920,"Height":1080},`+
		`{"Type":"%s","Offset":%d,"Length":%d,"Width":512,"Height":476}]}}]}`,
		lenScene, bodyType, lenScene, lenBody)
}

func TestDahuaPictureBase64List_imageArray_vehicle(t *testing.T) {
	scn, veh := jpeg("SCN", 200), jpeg("VEH", 130)
	blob := bytes.Join([][]byte{scn, veh}, nil)
	meta := metaImageArray("VehicleBody", len(scn), len(veh))
	ct, body := dahuaMultipart(t, meta, blob)
	var raw map[string]any
	_ = json.Unmarshal([]byte(meta), &raw)

	got := decodePics(t, dahuaPictureBase64List(ct, body, raw))
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Fatalf("want [scene, vehicleBody], got %d boxes", len(got))
	}
}

func TestDahuaPictureBase64List_imageArray_human(t *testing.T) {
	scn, hum := jpeg("SCN", 220), jpeg("HUM", 105)
	blob := bytes.Join([][]byte{scn, hum}, nil)
	meta := metaImageArray("HumanImage", len(scn), len(hum))
	ct, body := dahuaMultipart(t, meta, blob)
	var raw map[string]any
	_ = json.Unmarshal([]byte(meta), &raw)

	got := decodePics(t, dahuaPictureBase64List(ct, body, raw))
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], hum) {
		t.Fatalf("want [scene, humanImage], got %d boxes", len(got))
	}
}

func TestDahuaPictureBase64List_imageArray_picksBodyOverOtherCrop(t *testing.T) {
	scn, plate, veh := jpeg("SCN", 100), jpeg("PLT", 20), jpeg("VEH", 80)
	blob := bytes.Join([][]byte{scn, plate, veh}, nil)
	// Order in array: scene, a non-body crop (Plate), then the body crop.
	meta := fmt.Sprintf(`{"Events":[{"Data":{"Image":[`+
		`{"Type":"SceneImage","Offset":0,"Length":%d},`+
		`{"Type":"PlateImage","Offset":%d,"Length":%d},`+
		`{"Type":"VehicleBody","Offset":%d,"Length":%d}]}}]}`,
		len(scn), len(scn), len(plate), len(scn)+len(plate), len(veh))
	ct, body := dahuaMultipart(t, meta, blob)
	var raw map[string]any
	_ = json.Unmarshal([]byte(meta), &raw)

	got := decodePics(t, dahuaPictureBase64List(ct, body, raw))
	if len(got) != 2 || !bytes.Equal(got[0], scn) || !bytes.Equal(got[1], veh) {
		t.Fatalf("want [scene, vehicleBody] (plate skipped), got %d boxes", len(got))
	}
}

func TestDahuaPictureBase64List_noEventsReturnsNil(t *testing.T) {
	ct, body := dahuaMultipart(t, `{"Ack":true}`, jpeg("X", 50))
	var raw map[string]any
	_ = json.Unmarshal([]byte(`{"Ack":true}`), &raw)
	if pics := dahuaPictureBase64List(ct, body, raw); pics != nil {
		t.Fatalf("want nil, got %d pics", len(pics))
	}
}
