// controllers/ingestapi/requestBody_test.go
package ingestapi

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// captureBody mounts readIngestBody behind a Fiber route and returns what it
// produced for the given request.
func captureBody(t *testing.T, contentEncoding string, raw []byte) []byte {
	t.Helper()
	app := fiber.New()
	var got []byte
	app.Post("/x", func(c fiber.Ctx) error {
		got = append([]byte(nil), readIngestBody(c)...)
		return c.SendStatus(fiber.StatusAccepted)
	})

	req := httptest.NewRequest("POST", "/x", bytes.NewReader(raw))
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return got
}

// The Dahua regression: a plain body advertised as deflate must NOT become the
// fabricated "zlib: invalid header" string — it must pass through as raw.
func TestReadIngestBody_DahuaSpuriousDeflate(t *testing.T) {
	plain := []byte("--myboundary\r\nContent-Type: text/plain\r\n\r\nCode=TrafficJunction;data={\"plate\":\"กก1234\"}\r\n--myboundary--")
	got := captureBody(t, "deflate", plain)
	if string(got) != string(plain) {
		t.Fatalf("spurious deflate must pass raw through.\n got: %q\nwant: %q", got, plain)
	}
	if string(got) == "zlib: invalid header" {
		t.Fatal("regression: body became the Fiber decompress error string")
	}
}

func TestReadIngestBody_NoEncodingPassThrough(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	if got := captureBody(t, "", body); string(got) != string(body) {
		t.Fatalf("no encoding: got %q want %q", got, body)
	}
}

func TestReadIngestBody_RealGzipDecodes(t *testing.T) {
	want := []byte(`{"event":"anpr","plate":"1กข2345"}`)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(want)
	_ = zw.Close()

	if got := captureBody(t, "gzip", buf.Bytes()); string(got) != string(want) {
		t.Fatalf("gzip decode: got %q want %q", got, want)
	}
}

func TestReadIngestBody_RealZlibDeflateDecodes(t *testing.T) {
	want := []byte(`{"event":"ivs"}`)
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(want)
	_ = zw.Close()

	if got := captureBody(t, "deflate", buf.Bytes()); string(got) != string(want) {
		t.Fatalf("zlib deflate decode: got %q want %q", got, want)
	}
}

// sanity: tryInflate reports failure (not a panic) on non-compressed input.
func TestTryInflate_FailsCleanly(t *testing.T) {
	if _, ok := tryInflate([]byte("not compressed"), func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) }); ok {
		t.Fatal("expected zlib inflate to fail on plain bytes")
	}
}
