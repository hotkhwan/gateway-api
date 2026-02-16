package mapsvc

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// GenerateKmlSignedURL handles presigned KML file link generation
func GenerateKmlSignedURL(c *fiber.Ctx, token string) error {
	// ใช้ ctx จาก Fiber เพื่อสืบทอด trace/deadline
	baseCtx := c.UserContext()

	ctx, end, log := traceutil.StartLite(
		baseCtx,
		"github.com/hotkhwan/gateway-api/mapsvc",
		"maps.GenerateKmlSignedURL",
		"mapsvc", "GenerateKmlSignedURL",
	)
	defer end()

	// timeout เหมาะกับ I/O สั้น ๆ
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filename := c.Query("filename")
	expiresIn := c.QueryInt("expiresIn", 3600)

	log.Info().
		Str("filename", filename).
		Int("expiresIn", expiresIn).
		Msg("📥 Received request to generate signed KML URL")

	if filename == "" {
		log.Warn().Msg("⚠️ Missing 'filename' query parameter")
		return httputil.FailBadRequest(c, "MISSING_FILENAME", "Missing 'filename' query param")
	}

	// ✅ คงแนวทางเดียวกับ upload/list: ใช้ alias "map" ก่อน, fallback env/ค่าคงที่
	bucket := config.S3PublicBuckets["map"]
	if bucket == "" {
		bucket = os.Getenv("S3_BUCKET")
	}
	if bucket == "" {
		if v := os.Getenv("PROJECT"); v != "" {
			bucket = v
		} else {
			bucket = "klynx"
		}
	}

	objectKey := fmt.Sprintf("maps/kml/%s", filename)
	log.Debug().
		Str("bucket", bucket).
		Str("objectKey", objectKey).
		Msg("🔑 Preparing presigned URL")

	// แนะให้ set response content-type เป็น KML เพื่อ browser/clients รับถูกชนิด
	q := make(url.Values)
	q.Set("response-content-type", "application/vnd.google-earth.kml+xml")

	u, err := config.S3Client.PresignedGetObject(ctx, bucket, objectKey, time.Duration(expiresIn)*time.Second, q)
	if err != nil {
		log.Error().
			Err(err).
			Str("bucket", bucket).
			Str("objectKey", objectKey).
			Msg("❌ Failed to generate presigned URL")
		return httputil.FailInternal(c, "internal server error")
	}

	log.Info().Str("signedUrl", u.String()).Msg("✅ Presigned KML URL generated successfully")

	return gmod.SendSuccess(c, fiber.Map{
		"message":   "Presigned URL generated successfully",
		"signedUrl": u.String(),
		"expiresIn": fmt.Sprintf("%d seconds", expiresIn),
		"status":    true,
	})
}
