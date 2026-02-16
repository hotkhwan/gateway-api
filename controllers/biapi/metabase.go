// controllers/biapi/metabase.go
package biapi

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"

	"github.com/hotkhwan/gateway-api/internal/services/bisvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
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
	ctx := c.UserContext()
	tr := otel.Tracer("github.com/hotkhwan/gateway-api/biapi")
	ctx, span := tr.Start(ctx, "BI.SignBIUrl")
	defer span.End()

	did := c.Query("dashboardId", "")
	if did == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.BadRequestResponse{
			Code: "ERROR", Message: "invalid dashboardId", Status: false,
		})
	}

	res, err := bisvc.GenerateSignedURL(ctx, did)
	if err != nil {
		msg := "failed to sign token"
		if err == bisvc.ErrConfigNotSet {
			msg = "metabase env not configured"
		}
		return c.Status(http.StatusInternalServerError).JSON(gmod.BaseErrorResponse{
			BaseResponse: gmod.BaseResponse{Code: "ERROR", Message: msg, Status: false},
		})
	}

	return c.JSON(gmod.SuccessDataResponse{
		Code: "SUCCESS", Message: "Signed Metabase URL generated", Status: true, Data: res,
	})
}
