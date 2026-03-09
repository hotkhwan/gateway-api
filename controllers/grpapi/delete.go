// controllers/grpapi/delete.go
package grpapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeleteGroup godoc
// @Summary      Delete group
// @Description  Delete a group by id. Use ?cascade=detach to detach children to root (parentId=null). Default blocks if children exist.
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        id       path   string true  "Group ID"
// @Param        cascade  query  string false "detach | delete (delete is not implemented and will be rejected)"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      400 {object} gmod.BadRequestResponse
// @Failure      404 {object} gmod.ErrorMessageResponse
// @Failure      409 {object} gmod.ErrorMessageResponse "Has children"
// @Failure      500 {object} gmod.InternalErrorResponse
// @Router       /groups/{id} [delete]
// @Security     BearerAuth
func DeleteGroup(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.grpapi", "group.DeleteGroup", "grpapi", "DeleteGroup")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "MISSING_ID", "Group id is required")
	}
	cascade := c.Query("cascade") // "", "detach", "delete"

	if err := grpsvc.DeleteGroup(ctx, id, cascade); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to delete group")
		switch gmod.ErrCodeOf(err) { // ✅ ตอนนี้มีแล้วจากข้อ (1)
		case "GROUP_NOT_FOUND":
			return httputil.FailNotFound(c, "GROUP_NOT_FOUND", "Group not found")
		case "HAS_CHILDREN":
			return httputil.FailConflict(c, "HAS_CHILDREN", "Group has child groups. Use ?cascade=detach to detach children.")
		case "UNSUPPORTED_CASCADE":
			return httputil.FailBadRequest(c, "UNSUPPORTED_CASCADE", "cascade=delete is not supported")
		default:
			return httputil.FailInternal(c, "DELETE_FAILED", err.Error())
		}
	}

	return httputil.MessageOK(c, "Group deleted successfully", "GROUP_DELETED")
}
