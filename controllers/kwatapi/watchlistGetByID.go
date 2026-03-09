// controllers/kwatapi/watchlistGetByID.go
package kwatapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// watchlistGetByID godoc
// @Summary Get watchlist by ID
// @Description Return a watchlist document by ID (soft-delete aware)
// @Tags Kwatch/Watchlist
// @Produce json
// @Param id path string true "Watchlist ID (ID or IDCard)"
// @Success 200 {object} object{detail=kwatmod.WatchlistResponse,status=bool}
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Router /kwatch/watchlist/{id} [get]
// @Security BearerAuth
func WatchlistGetByID(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.UserContext(), "gateway.kwatapi", "Watchlist.WatchlistGetByID", "kwatapi", "WatchlistGetByID")
	defer end()

	id := c.Params("id") // <-- ใช้ได้ทั้ง ObjectID และ idcard
	data, err := kwatsvc.WatchlistGetByID(ctx, id)
	if err != nil {
		return httputil.FailNotFound(c, "Watchlist not found")
	}
	return httputil.Ok(c, data, "Watchlist retrieved")
}
