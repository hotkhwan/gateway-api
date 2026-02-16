// controllers/kwatapi/watchlistGetByID.go
package kwatapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
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
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/kwatapi")
	ctx, span := tracer.Start(ctx, "Kwatch.WatchlistGetByID")
	defer span.End()

	id := c.Params("id") // <-- ใช้ได้ทั้ง ObjectID และ idcard
	data, err := kwatsvc.WatchlistGetByID(ctx, id)
	if err != nil {
		return httputil.FailNotFound(c, "NOT_FOUND", "Watchlist not found")
	}
	return gmod.SendSuccess(c, data)
}
