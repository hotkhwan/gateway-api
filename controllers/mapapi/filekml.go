// controllers/mapapi/kml_file.go
package mapapi

import (
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// GetKMLFile godoc
// @Summary Get presigned KML file URL
// @Tags Maps
// @Accept json
// @Produce json
// @Param filename query string true "File name"
// @Param expiresIn query int false "Expire time in seconds"
// @Success 200 {object} gmod.PaginationResponse
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /maps/kml/file [get]
// @Security BearerAuth
func GetKMLFile(c *fiber.Ctx) error {
	format := c.Query("format", "kml") // default = kml
	logger := log.With().Str("traceID", c.Get("X-Trace-ID")).Logger()
	logger.Info().
		Str("format", format).
		Msg("📤 [ExportAlarms] Export request received")
	return gmod.SendMessageOK(c, "GET_KMLFILE_SUCCESS", "Get kml file successfully")
}
