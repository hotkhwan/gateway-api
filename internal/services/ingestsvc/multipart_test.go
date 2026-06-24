// internal/services/ingestsvc/multipart_test.go
package ingestsvc

import (
	"encoding/json"
	"strings"
	"testing"
)

// buildMultipart assembles a multipart/x-mixed-replace body from raw parts.
func buildMultipart(boundary string, parts []struct {
	ctype string
	body  string
}) []byte {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("--" + boundary + "\r\n")
		if p.ctype != "" {
			b.WriteString("Content-Type: " + p.ctype + "\r\n")
		}
		b.WriteString("\r\n")
		b.WriteString(p.body)
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

func TestNormalizeMultipart_JSONMetaPlusImage(t *testing.T) {
	body := buildMultipart("myboundary", []struct {
		ctype string
		body  string
	}{
		{"application/json", `{"Plate":"1กข2345","VehicleType":"Sedan","Color":"White"}`},
		{"image/jpeg", "\xff\xd8\xff\xe0BIGBINARYJPEGDATA...."},
	})

	out, ok := normalizeMultipart("multipart/x-mixed-replace; boundary=myboundary", body)
	if !ok {
		t.Fatal("expected multipart to parse")
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if m["Plate"] != "1กข2345" || m["VehicleType"] != "Sedan" {
		t.Fatalf("metadata fields missing: %v", m)
	}
	bins, _ := m["_binaries"].([]any)
	if len(bins) != 1 {
		t.Fatalf("expected 1 binary descriptor, got %v", m["_binaries"])
	}
	// The JPEG bytes must NOT be inlined anywhere in the output.
	if strings.Contains(string(out), "BIGBINARYJPEGDATA") {
		t.Fatal("binary payload leaked into normalized event")
	}
}

func TestNormalizeMultipart_DahuaKVWithData(t *testing.T) {
	meta := `Code=TrafficJunction;action=Pulse;index=0;data={"Plate":{"PlateNumber":"กก1234"},"Speed":58}`
	body := buildMultipart("myboundary", []struct {
		ctype string
		body  string
	}{
		{"text/plain", meta},
		{"image/jpeg", "\xff\xd8\xff\xe0snapshot"},
	})

	out, ok := normalizeMultipart("multipart/x-mixed-replace; boundary=myboundary", body)
	if !ok {
		t.Fatal("expected parse")
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)

	if m["Code"] != "TrafficJunction" || m["action"] != "Pulse" {
		t.Fatalf("leading KV not parsed: %v", m)
	}
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("data not parsed as object: %v", m["data"])
	}
	plate, _ := data["Plate"].(map[string]any)
	if plate["PlateNumber"] != "กก1234" {
		t.Fatalf("nested plate missing: %v", data)
	}
}

func TestNormalizeMultipart_UnstructuredKeptAsText(t *testing.T) {
	body := buildMultipart("b", []struct {
		ctype string
		body  string
	}{
		{"text/plain", "Heartbeat"},
	})
	out, ok := normalizeMultipart("multipart/x-mixed-replace; boundary=b", body)
	if !ok {
		t.Fatal("expected parse")
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	txt, _ := m["_text"].([]any)
	if len(txt) != 1 || txt[0] != "Heartbeat" {
		t.Fatalf("unstructured text not preserved: %v", m)
	}
}

func TestIsMultipartContentType(t *testing.T) {
	cases := map[string]bool{
		"multipart/x-mixed-replace; boundary=myboundary": true,
		"multipart/form-data; boundary=x":                true,
		"application/json":                               false,
		"text/plain":                                     false,
		"":                                               false,
	}
	for ct, want := range cases {
		if got := isMultipartContentType(ct); got != want {
			t.Errorf("isMultipartContentType(%q)=%v want %v", ct, got, want)
		}
	}
}

func TestNormalizeMultipart_NotMultipart(t *testing.T) {
	if _, ok := normalizeMultipart("application/json", []byte(`{"a":1}`)); ok {
		t.Fatal("non-multipart content type must not parse as multipart")
	}
}
