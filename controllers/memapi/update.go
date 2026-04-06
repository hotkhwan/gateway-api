package memapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/memsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/memmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// UpdateMember godoc
// @Summary Update member by ID
// @Description Update member detail by ID
// @Tags members
// @Produce  json
// @Param id path string true "member ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members/{id} [put]
// @Security BearerAuth
func UpdateMember(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.memapi", "memapi.UpdateMember", "memapi", "UpdateMember")
	defer end()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "MISSING_ID", "Group id is required")
	}

	var req memmod.MemberRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Error().Err(err).Msg("❌ Invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
	}

	if err := memsvc.UpdateMember(ctx, id, req); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to update group")
		switch gmod.ErrCodeOf(err) { // ✅ ตอนนี้มีแล้วจากข้อ (1)
		case "GROUP_NOT_FOUND":
			return httputil.FailNotFound(c, "GROUP_NOT_FOUND", "Group not found")
		case "PARENT_NOT_FOUND":
			return httputil.FailBadRequest(c, "PARENT_NOT_FOUND", "Parent group not found")
		case "CIRCULAR_PARENT":
			return httputil.FailConflict(c, "CIRCULAR_PARENT", "Circular parent assignment is not allowed")
		default:
			return httputil.FailInternalReason(c, "internal server error", "UPDATE_FAILED")
		}
	}

	return httputil.MessageOK(c, "Group updated successfully", "GROUP_UPDATED")

}
