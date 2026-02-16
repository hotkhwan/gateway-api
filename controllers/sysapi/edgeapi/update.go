// controllers/sysapi/edgeapi/update.go
package edgeapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/edgesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/systemmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// UpdateEdge godoc
// @Summary Update edge by id (encrypt secrets if provided)
// @Tags System Edge
// @Accept json
// @Produce json
// @Param id path string true "edge id"
// @Param body body systemmod.EdgeUpdateReq true "payload"
// @Success 200 {object} systemmod.EdgeUpdateSuccessResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /system/edge/{id} [patch]
// @Security BearerAuth
func UpdateEdge(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/edgeapi", "system.edge.Update", "edgeapi", "UpdateEdge")
	defer span.End()

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return httputil.FailBadRequestReason(c, "missing id", "MISSING_ID")
	}

	var req systemmod.EdgeUpdateReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid body")
		return httputil.FailBadRequestReason(c, "invalid json body", "INVALID_BODY")
	}

	// ✅ กัน request ว่าง ๆ (ไม่มี field ไหนมาเลย)
	if req.Username == nil && req.Password == nil && req.URL == nil && req.TLS == nil && req.APIKey == nil && req.APISecret == nil {
		return httputil.FailBadRequestReason(c, "no fields to update", "EMPTY_UPDATE")
	}

	if err := edgesvc.UpdateEdge(ctx, id, req); err != nil {
		// ถ้าคุณมี sentinel error ดีกว่า string match มาก (แนะนำทำทีหลัง)
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return httputil.FailNotFound(c, "edge not found", httputil.ErrorDetails{
				"reason": "NOT_FOUND",
			})
		}

		log.Error().Err(err).Msg("update edge failed")
		// ไม่ leak err.Error() ออกไป
		return httputil.FailInternalReason(c, "internal server error", "UPDATE_FAILED")
	}

	return c.JSON(systemmod.EdgeUpdateSuccessResponse{
		Code:    gmod.CodeSuccess,
		Message: "edge updated",
		Status:  true,
	})
}
