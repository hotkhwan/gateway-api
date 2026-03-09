// controllers/mapapi/uploadkml.go
package mapapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/mapsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
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
	_, end, log := traceutil.StartLite(c.UserContext(), "gateway.mapapi", "UploadKML.UploadKML", "mapapi", "UploadKML")
	defer end()

	token := c.Get("Authorization")
	if token == "" {
		log.Warn().Msg("Missing Authorization header")
		return httputil.FailUnauthorized(c, "Missing Authorization header")
	}

	return mapsvc.HandleKMLUpload(c, token)
}
