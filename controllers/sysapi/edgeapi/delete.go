// controllers/sysapi/edgeapi/delete.go
package edgeapi

import (
	"strings"

	"klynx/internal/logger"
	"klynx/internal/services/systemsvc/edgesvc"
	"klynx/models/systemmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeleteEdge godoc
// @Summary Soft delete edge by id
// @Tags System Edge
// @Accept json
// @Produce json
// @Param id path string true "edge id"
// @Success 200 {object} systemmod.EdgeDeleteSuccessResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 404 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /system/edge/{id} [delete]
// @Security BearerAuth
func DeleteEdge(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/edgeapi", "system.edge.Delete", "edgeapi", "DeleteEdge")
	defer span.End()
	_ = logger.FromCtx(ctx, "edgeapi", "DeleteEdge")

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return httputil.FailBadRequest(c, "INVALID_ID", "missing id")
	}

	// ✅ ดึงผู้ใช้จาก Locals("user") ของ AuthBearer (audit ของคุณก็ทำคล้าย ๆ นี้)
	deletedBy := "unknown"
	if u := c.Locals("user"); u != nil {
		if m, ok := u.(map[string]any); ok {
			if s, ok := m["preferred_username"].(string); ok && strings.TrimSpace(s) != "" {
				deletedBy = s
			} else if s, ok := m["sub"].(string); ok && strings.TrimSpace(s) != "" {
				deletedBy = s
			}
		}
	}

	if err := edgesvc.SoftDeleteEdge(ctx, id, deletedBy); err != nil {
		// SoftDeleteMany ถ้า filter ไม่ match จะไม่ error → แต่เรายังถือว่าควรตอบ 404 ได้
		// วิธีง่าย: ถ้าคุณอยาก strict เพิ่ม matchedCount ต้องใช้ UpdateOne
		// ตอนนี้ใช้ not found message แบบ conservative:
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return httputil.FailBadRequest(c, "NOT_FOUND", "edge not found")
		}
		log.Error().Err(err).Msg("delete edge failed")
		return httputil.FailInternalReason(c, "internal server error", "DELETE_FAILED")
	}

	return c.JSON(systemmod.EdgeDeleteSuccessResponse{
		Code:    "SUCCESS",
		Message: "edge deleted",
		Status:  true,
	})
}
