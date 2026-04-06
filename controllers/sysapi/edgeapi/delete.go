// controllers/sysapi/edgeapi/delete.go
package edgeapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/edgesvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
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
func DeleteEdge(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.edgeapi", "EdgeApi.DeleteEdge", "sysapi", "DeleteEdge")
	defer end()

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return httputil.FailBadRequest(c, "missing id")
	}

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
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return httputil.FailNotFound(c, "edge not found")
		}
		log.Error().Err(err).Msg("delete edge failed")
		return httputil.FailInternalReason(c, "internal server error", "DELETE_FAILED")
	}

	return httputil.MessageOK(c, "edge deleted")
}
