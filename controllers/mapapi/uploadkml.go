// controllers/mapapi/uploadkml.go
package mapapi

import (
	"klynx/internal/services/mapsvc"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// UploadKML godoc
// @Summary Upload and validate KML file
// @Tags Maps
// @Accept multipart/form-data
// @Produce json
// @Param description formData string false "Description of the KML"
// @Param file formData file true "KML file"
// @Success 200 {object} gmod.PaginationResponse
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 401 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /maps/kml [post]
// @Security BearerAuth
func UploadKML(c *fiber.Ctx) error {
	token := c.Get("Authorization")
	if token == "" {
		log.Warn().Msg("Missing Authorization header")
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", "Missing Authorization header")
	}

	return mapsvc.HandleKMLUpload(c, token)
}
