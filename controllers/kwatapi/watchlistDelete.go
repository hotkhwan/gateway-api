// controllers/kwatapi/watchlistDelete.go
package kwatapi

import (
	"errors"

	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// watchlistDelete godoc
// @Summary Delete a Watchlist by ID or ID Card
// @Description Soft delete + downstream (external/S3) via event. รับเป็น **_id หรือ idcard** ผ่าน path param เดียวกัน
// @Tags Kwatch/Watchlist
// @Produce json
// @Param   id  path string true "Watchlist _id หรือ idcard"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ErrorMessageResponse "Invalid ID"
// @Failure 404 {object} gmod.ErrorMessageResponse "Watchlist not found"
// @Failure 500 {object} gmod.ErrorMessageResponse "Internal Server Error"
// @Router /kwatch/watchlist/{id} [delete]
// @Security BearerAuth
func WatchlistDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kwatapi", "Watchlist.WatchlistDelete", "kwatapi", "WatchlistDelete")
	defer end()

	idOrIdCard := c.Params("id")
	if idOrIdCard == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "id (watchlist _id หรือ idcard) is required")
	}

	// ปล่อยให้ service แปลเองว่าเป็น _id หรือ idcard
	if err := kwatsvc.WatchlistDelete(ctx, idOrIdCard); err != nil {
		log.Error().Err(err).Str("idOrIdCard", idOrIdCard).Msg("❌ Failed to delete watchlist")
		switch {
		case errors.Is(err, kwatsvc.ErrInvalidObjectID):
			// หมายเหตุ: จะเจอกรณีนี้เฉพาะตอนส่งเป็น _id แล้ว parse ไม่ได้
			return httputil.FailBadRequest(c, "INVALID_ID", "Invalid watchlist ID format")
		case errors.Is(err, kwatsvc.ErrWatchlistNotFound):
			return httputil.FailNotFound(c, "NOT_FOUND", "Watchlist not found by _id or idcard")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to delete watchlist")
		}
	}

	return httputil.MessageOK(c, "Watchlist deleted successfully")
}
