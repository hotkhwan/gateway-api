// controllers/sysapi/edgeapi/edgeSsoUrl.go
package edgeapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/edgesvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// GetEdgeSSOURL godoc
// @Summary Get edge SSO URL (decrypt passEnc then base64 encode password)
// @Tags System Edge
// @Accept json
// @Produce json
// @Param edgeType path string true "edge type" Enums(ata,svms,iboc)
// @Param id path string true "Mongo ObjectId of edge"
// @Param address query string false "address target (default play)" default(play)
// @Success 200 {object} systemmod.EdgeSSOURLSuccessResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 404 {object} gmod.NotFoundResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /system/edge/{edgeType}/sso/{id} [get]
// @Security BearerAuth
func GetEdgeSSOURL(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.edgeapi", "EdgeApi.GetEdgeSSOURL", "sysapi", "GetEdgeSSOURL")
	defer end()

	edgeType := strings.ToLower(strings.TrimSpace(c.Params("edgeType")))
	id := strings.TrimSpace(c.Params("id"))
	address := strings.TrimSpace(c.Query("address", "play"))

	if edgeType == "" || id == "" {
		return httputil.FailBadRequest(c, "edgeType and id are required")
	}

	ssoUrl, err := edgesvc.BuildEdgeSSOURL(ctx, edgeType, id, address)
	if err != nil {
		switch {
		case edgesvc.IsUnsupportedEdgeType(err):
			return httputil.FailBadRequest(c, err.Error())
		case edgesvc.IsNotFound(err):
			return httputil.FailNotFound(c, "edge not found")
		case edgesvc.IsDeleted(err):
			return httputil.FailNotFound(c, "edge not found")
		default:
			log.Error().Err(err).Str("edgeType", edgeType).Str("id", id).Msg("build sso url failed")
			return httputil.FailInternalReason(c, "internal server error", "SSO_URL_BUILD_FAILED")
		}
	}

	return httputil.Ok(c, fiber.Map{
		"ssoUrl": ssoUrl,
	})
}
