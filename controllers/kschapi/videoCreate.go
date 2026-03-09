// controllers/kschapi/videoCreate.go
package kschapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

const maxVideoBytes = int64(10 * 1024 * 1024 * 1024) // 10GB

// videoCreate godoc
// @Summary Upload a Video
// @Description Upload a video file (<= 10GB). Supported types: mp4, avi, mov, mkv
// @Tags Ksearch/Video
// @Accept multipart/form-data
// @Produce json
// @Param name formData string true "Video name"
// @Param description formData string false "Description"
// @Param status formData string false "true/false (default: true)"
// @Param state formData string false "Initial state (default: created)"
// @Param video formData file true "Video file (mp4/avi/mov/mkv)"
// @Success 201 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ErrorMessageResponse
// @Failure 413 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.ErrorMessageResponse
// @Router /ksearch/video [post]
// @Security BearerAuth
func VideoCreate(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.kschapi", "Video.VideoCreate", "kschapi", "VideoCreate")
	defer end()

	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Minute)
	defer cancel()

	name := c.FormValue("name")
	if name == "" {
		name = c.FormValue("type")
	}
	description := c.FormValue("description")
	if description == "" {
		description = c.FormValue("describtion")
	}
	status := c.FormValue("status")
	state := c.FormValue("state")

	fh, err := c.FormFile("video")
	if err != nil || fh == nil {
		fh, err = c.FormFile("file")
		if err != nil || fh == nil {
			return httputil.FailBadRequest(c, "VIDEO_REQUIRED", "Video file is required")
		}
	}
	if fh.Size <= 0 {
		return httputil.FailBadRequest(c, "VIDEO_REQUIRED", "Video file is required")
	}
	if fh.Size > maxVideoBytes {
		return httputil.FailBadRequest(c, "FILE_TOO_LARGE", "File too large (max 10GB)")
	}

	f, err := fh.Open()
	if err != nil {
		return httputil.FailBadRequest(c, "BAD_REQUEST", fmt.Sprintf("open file error: %v", err))
	}
	defer f.Close()

	reader := io.MultiReader(bytes.NewReader(nil), f)

	req := kschmod.VideoCreateRequest{
		Name:             name,
		Description:      description,
		Status:           status,
		State:            state,
		VideoR:           reader,
		VideoSize:        fh.Size,
		VideoFileName:    fh.Filename,
		VideoContentType: fh.Header.Get("Content-Type"),
	}

	id, err := kschsvc.VideoCreate(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, kschsvc.ErrVideoRequired):
			return httputil.FailBadRequest(c, "VIDEO_REQUIRED", "Video file is required")
		case errors.Is(err, kschsvc.ErrFileTooLarge):
			return httputil.FailBadRequest(c, "FILE_TOO_LARGE", "File too large (max 10GB)")
		case errors.Is(err, kschsvc.ErrUnsupportedVideoType):
			return httputil.FailBadRequest(c, "UNSUPPORTED_VIDEO_TYPE", "Only mp4, avi, mov, mkv are allowed")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to create video")
		}
	}

	// ✅ ใช้ SuccessMessageCreateResponse (top-level id)
	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessMessageCreateResponse{
		Code:    "SUCCESS",
		Message: "Video created successfully",
		Status:  true,
		ID:      id.Hex(),
	})
}
