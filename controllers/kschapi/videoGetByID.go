// controllers/kschapi/videoGetByID.go
package kschapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// videoGetByID godoc
// @Summary Get video by ID
// @Description Get a single video document by ID
// @Tags Ksearch/Video
// @Produce json
// @Param id path string true "video ID"
// @Success 200 {object} object{detail=kschmod.VideoResponse,status=bool}
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Router /kschapi/video/{id} [get]
// @Security BearerAuth
func VideoGetByID(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "Video.VideoGetByID", "kschapi", "VideoGetByID")
	defer end()

	videoId := c.Params("videoId")
	data, err := kschsvc.VideoGetByID(ctx, videoId)
	if err != nil {
		log.Error().Err(err).Str("id", videoId).Msg("❌ Failed to get watchlist")
		return httputil.FailNotFound(c, "NOT_FOUND", "Watchlist not found")
	}

	return httputil.Ok(c, data)
}
