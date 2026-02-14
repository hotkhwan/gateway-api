// controllers/fileapi/imageProxy.go
package fileapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"klynx/internal/repo/stos3minio"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// controllers/fileapi/filesProxy.go
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
// @Router       /image/{key} [get]
func ProxyFiles(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"klynx/fileapi",
		"file.ProxyFiles",
		"fileapi",
		"ProxyFiles",
	)
	defer span.End()

	raw := strings.TrimSpace(c.Params("*"))
	if raw == "" {
		return c.Status(fiber.StatusBadRequest).SendString("missing key")
	}

	// raw: "ata-feature/images/60038008431d6040/6937d7480fcda07b918bfa13/0.jpg"
	raw = strings.TrimLeft(raw, "/")
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		log.Warn().Str("raw", raw).Msg("invalid image path (need bucket/key)")
		return c.Status(fiber.StatusBadRequest).SendString("invalid image path")
	}

	bucket := parts[0]
	key := parts[1] // ✅ ใช้ส่วนหลังเป็น key จริง

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
			Msg("❌ DownloadByKey failed")
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
