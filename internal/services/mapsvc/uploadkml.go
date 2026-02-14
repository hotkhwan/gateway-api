package mapsvc

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"klynx/internal/logger"
	"klynx/internal/repo/stos3minio"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"
)

func HandleKMLUpload(c *fiber.Ctx, token string) error {
	baseCtx := c.UserContext()
	ctx, end, log := traceutil.StartLite(
		baseCtx,
		"klynx/mapsvc",
		"maps.HandleKMLUpload",
		"mapsvc", "HandleKMLUpload",
	)
	defer end()

	// กัน upload ใหญ่เกิน/ช้าจนค้าง
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	log = logger.WithMeta("mapsvc", "HandleKMLUpload")
	log.Info().Msg("📥 Received KML upload request")

	form, err := c.MultipartForm()
	if err != nil || len(form.File["file"]) == 0 {
		log.Warn().Msg("⚠️ No file uploaded")
		return httputil.FailBadRequest(c, "NO_FILE", "No file uploaded.")
	}

	fileHeader := form.File["file"][0]
	description := c.FormValue("description", "No description provided")
	ext := strings.ToLower(fileHeader.Filename[strings.LastIndex(fileHeader.Filename, ".")+1:])
	fileType := ext

	log.Debug().
		Str("filename", fileHeader.Filename).
		Str("fileType", fileType).
		Str("description", description).
		Msg("📄 File metadata received")

	if fileType != "kml" {
		log.Warn().Str("fileType", fileType).Msg("⚠️ Invalid file type")
		return httputil.FailBadRequest(c, "INVALID_TYPE", "Unsupported file type. Only .kml allowed.")
	}

	f, err := fileHeader.Open()
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to open uploaded file")
		return httputil.FailBadRequest(c, "FILE_READ_ERROR", "Failed to read uploaded file.")
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to read file buffer")
		return httputil.FailBadRequest(c, "BUFFER_READ_ERROR", "Failed to read file buffer.")
	}

	xml := string(buf)
	if !strings.Contains(xml, "<kml") || !strings.Contains(xml, "</kml>") {
		log.Warn().Msg("⚠️ Invalid KML format")
		return httputil.FailBadRequest(c, "INVALID_FORMAT", "Invalid KML file format.")
	}

	objectKey := fmt.Sprintf("maps/kml/%s.kml", uuid.New().String())
	encodedDescription := base64.StdEncoding.EncodeToString([]byte(description))

	url, err := stos3minio.UploadWithMeta(ctx, "map", true, objectKey, buf, "application/vnd.google-earth.kml+xml", map[string]string{
		"filetype":    fileType,
		"description": encodedDescription,
	})
	if err != nil {
		log.Error().Err(err).Str("key", objectKey).Msg("❌ Upload to S3 failed")
		return httputil.FailInternal(c, "internal server error")
	}

	log.Info().Str("path", url).Msg("✅ Upload successful")

	return gmod.SendSuccess(c, fiber.Map{
		"message": "Upload successful",
		"path":    url,
		"status":  true,
	})
}
