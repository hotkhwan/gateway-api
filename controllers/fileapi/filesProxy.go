// controllers/fileapi/filesProxy.go
package fileapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// GetImage godoc
// @Summary      Proxy image from S3/MinIO
// @Description  Proxy private image from S3/MinIO โดย key จะเป็น path (ต้อง URL-encode ถ้ามี / หรือ space)
// @Tags         Files
// @Produce      octet-stream
// @Param        key   path      string  true  "Object key (URL-encoded, can contain /)"
// @Success      200   {file}    file
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      404   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /files/{key} [get]
func ProxyFiles(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.fileapi", "file.ProxyFiles", "fileapi", "ProxyFiles")
	defer end()

	raw := strings.TrimSpace(c.Params("*"))
	if raw == "" {
		return httputil.FailBadRequest(c, "missing key")
	}

	// Format A: "{bucket}/{key}"  e.g. "ata-feature/images/xxx/0.jpg"
	// Format B: "{workspaceId}/events/{eventId}/file.jpg" — bucket resolved from S3_BUCKET_EVENTS
	raw = strings.TrimLeft(raw, "/")
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		log.Warn().Str("raw", raw).Msg("invalid image path (need bucket/key or workspaceId/key)")
		return httputil.FailBadRequest(c, "invalid image path")
	}

	bucket, key := resolveFileBucketAndKey(parts[0], parts[1])

	// ถ้าอยาก safety เพิ่ม อาจ URL-decode ตรงนี้ก็ได้:
	// if decoded, err := url.PathUnescape(key); err == nil {
	//     key = decoded
	// }

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	data, filename, err := stos3minio.DownloadByKey(ctx, bucket, key)
	if err != nil {
		log.Error().
			Err(err).
			Str("bucket", bucket).
			Str("key", key).
			Msg("DownloadByKey failed")
		return c.SendStatus(fiber.StatusBadGateway)
	}

	contentType := detectContentType(filename, data)
	if contentType != "" {
		c.Set("Content-Type", contentType)
	}

	c.Set("Cache-Control", "public, max-age=86400")
	c.Set("Accept-Ranges", "bytes")
	return c.Status(http.StatusOK).Send(data)
}

// ProxyEventImage godoc
// @Summary      Proxy public image from S3
// @Description  Serves a binary from a public S3 bucket. No auth required. Path: {bucket}/{key}.
// @Tags         Files
// @Produce      octet-stream
// @Param        key   path   string  true  "S3 path: {publicBucket}/{objectKey}"
// @Success      200   {file} file
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      403   {object}  gmod.ErrorResponse
// @Failure      404   {object}  gmod.ErrorResponse
// @Router       /image/{key} [get]
func ProxyEventImage(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.fileapi", "file.ProxyEventImage", "fileapi", "ProxyEventImage")
	defer end()

	raw := strings.TrimLeft(strings.TrimSpace(c.Params("*")), "/")
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return httputil.FailBadRequest(c, "invalid image path (need {bucket}/{key})")
	}

	bucket, key := parts[0], parts[1]

	// Only public buckets — no auth so private buckets must not be accessible here.
	if _, ok := config.S3PublicBuckets[bucket]; !ok {
		log.Warn().Str("bucket", bucket).Msg("attempt to access non-public bucket via /image")
		return c.SendStatus(fiber.StatusForbidden)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	data, filename, err := stos3minio.DownloadByKey(ctx, bucket, key)
	if err != nil {
		log.Warn().Err(err).Str("bucket", bucket).Str("key", key).Msg("image not found")
		return c.SendStatus(fiber.StatusNotFound)
	}

	c.Set("Content-Type", detectContentType(filename, data))
	c.Set("Cache-Control", "public, max-age=86400")
	c.Set("Accept-Ranges", "bytes")
	return c.Status(http.StatusOK).Send(data)
}

// resolveFileBucketAndKey returns the real bucket name and object key.
// If firstSegment is a known S3 bucket name, it is used as-is (Format A).
// Otherwise the full path (firstSegment+"/"+rest) is used as the key
// and the bucket falls back to S3_BUCKET_EVENTS (Format B — event image paths).
func resolveFileBucketAndKey(firstSegment, rest string) (bucket, key string) {
	if isKnownBucket(firstSegment) {
		return firstSegment, rest
	}
	// Format B: workspaceId/events/… — use events bucket
	eventsBucket := os.Getenv("S3_BUCKET_EVENTS")
	if eventsBucket == "" {
		eventsBucket = "canonical"
	}
	return eventsBucket, firstSegment + "/" + rest
}

// isKnownBucket reports whether name is registered in the S3 public or private bucket maps.
func isKnownBucket(name string) bool {
	if _, ok := config.S3PublicBuckets[name]; ok {
		return true
	}
	_, ok := config.S3PrivateBuckets[name]
	return ok
}

func detectContentType(filename string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	}

	// fallback ใช้ sniff จาก content
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return ""
}
