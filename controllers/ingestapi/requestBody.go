// controllers/ingestapi/requestBody.go
package ingestapi

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// readIngestBody returns the request body for the ingest hot path WITHOUT
// Fiber's built-in Content-Encoding auto-decompression.
//
// Why: fiber v3 `c.Body()` inflates the body when a `Content-Encoding` header
// is present, and on failure it returns `[]byte(err.Error())` (see
// fiber/v3 req.go Body()). Dahua cameras POST a `multipart/x-mixed-replace`
// stream while advertising `Content-Encoding: deflate`, but the multipart
// payload is NOT zlib-wrapped — so `c.Body()` fabricates the literal 20-byte
// string "zlib: invalid header" as the body and the real event is lost.
//
// We instead take the raw bytes (`c.BodyRaw()`) and decode only when the data
// is genuinely compressed: zlib/gzip readers validate their header magic and
// fail fast on plain data, so a mislabeled or spurious Content-Encoding (the
// Dahua case) falls through to the untouched raw body — never a fabricated
// error string. Sources that send no Content-Encoding (AIBOX/PVS JSON) are
// unaffected: raw == body.
func readIngestBody(c fiber.Ctx) []byte {
	raw := c.BodyRaw()
	enc := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderContentEncoding)))
	if enc == "" || len(raw) == 0 {
		return raw
	}

	switch {
	case strings.Contains(enc, "gzip"):
		if out, ok := tryInflate(raw, func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) }); ok {
			return out
		}
	case strings.Contains(enc, "deflate"):
		// zlib header is validated; Dahua's uncompressed multipart fails this
		// and we keep the raw bytes (the correct, real payload).
		if out, ok := tryInflate(raw, func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) }); ok {
			return out
		}
	}

	return raw
}

// tryInflate runs raw through the reader built by mk, returning the decompressed
// bytes only on full success; any header/read error means "not actually
// compressed this way" and the caller keeps the raw body.
func tryInflate(raw []byte, mk func(io.Reader) (io.ReadCloser, error)) ([]byte, bool) {
	zr, err := mk(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}
