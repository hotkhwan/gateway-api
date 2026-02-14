// controllers/grpapi/update.go
package grpapi

import (
	"klynx/internal/services/grpsvc"
	"klynx/models/gmod"
	"klynx/models/grpmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// UpdateGroup godoc
// @Summary      Update group
// @Description  Partially update a group by id (name, parentId, description, icon)
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        id    path      string               true  "Group ID"
// @Param        group body      grpmod.GroupRequest  true  "Group data to update (partial)"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      404   {object}  gmod.ErrorMessageResponse
// @Failure      409   {object}  gmod.ErrorMessageResponse "Conflict (e.g., circular parent)"
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Router       /groups/{id} [patch]
// @Security     BearerAuth
func UpdateGroup(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/grpapi", "group.UpdateGroup", "grpapi", "UpdateGroup")
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "MISSING_ID", "Group id is required")
	}

	var req grpmod.GroupRequest
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ Invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
	}

	// guard: parentId == self
	if req.ParentID != nil && *req.ParentID == id {
		return httputil.FailBadRequest(c, "INVALID_PARENT", "parentId cannot be the same as the group id")
	}

	if err := grpsvc.UpdateGroup(ctx, id, req); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to update group")
		switch gmod.ErrCodeOf(err) { // ✅ ตอนนี้มีแล้วจากข้อ (1)
		case "GROUP_NOT_FOUND":
			return httputil.FailNotFound(c, "GROUP_NOT_FOUND", "Group not found")
		case "PARENT_NOT_FOUND":
			return httputil.FailBadRequest(c, "PARENT_NOT_FOUND", "Parent group not found")
		case "CIRCULAR_PARENT":
			return httputil.FailConflict(c, "CIRCULAR_PARENT", "Circular parent assignment is not allowed")
		default:
			return httputil.FailInternal(c, "UPDATE_FAILED", err.Error())
		}
	}

	// ⬇️ เปลี่ยนจาก SendSuccessMessage -> SendMessageOK (ที่มีอยู่แล้ว)
	return gmod.SendMessageOK(c, "GROUP_UPDATED", "Group updated successfully")
}
