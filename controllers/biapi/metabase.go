// controllers/biapi/metabase.go
package biapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/internal/services/bisvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// SignBIUrl godoc
// @Summary Generate signed Metabase embed URL
// @Description Return signed iframe URL (base URL only, no hash) for embedding Metabase dashboard
// @Tags BI
// @Accept  json
// @Produce  json
// @Param dashboardId query int true "Dashboard ID"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.BaseErrorResponse
// @Router /bi/signUrl [get]
// @Security BearerAuth
func SignBIUrl(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.biapi", "BI.SignBIUrl", "biapi", "SignBIUrl")
	defer end()

	did := c.Query("dashboardId", "")
	if did == "" {
		return httputil.FailBadRequest(c, "invalid dashboardId")
	}

	res, err := bisvc.GenerateSignedURL(ctx, did)
	if err != nil {
		msg := "failed to sign token"
		if err == bisvc.ErrConfigNotSet {
			msg = "metabase env not configured"
		}
		log.Error().Err(err).Msg(msg)
		return httputil.FailInternal(c, msg)
	}

	return httputil.Ok(c, res, "Signed Metabase URL generated")
}
