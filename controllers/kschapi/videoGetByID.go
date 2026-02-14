// controllers/kschapi/videoGetByID.go
package kschapi

import (
	"klynx/internal/services/kschsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
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
func VideoGetByID(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kschapi", "search.VideoGetByID", "kschapi", "VideoGetByID")
	defer span.End()

	videoId := c.Params("videoId")
	data, err := kschsvc.VideoGetByID(ctx, videoId)
	if err != nil {
		log.Error().Err(err).Str("id", videoId).Msg("❌ Failed to get watchlist")
		return httputil.FailNotFound(c, "NOT_FOUND", "Watchlist not found")
	}

	// ✅ read ใช้รูปแบบ SuccessDetailResponse<T>
	return gmod.SendSuccess(c, data)
	// return gmod.SendSuccess[kschmod.VideoResponse](c, data)
}
