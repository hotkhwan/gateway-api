package memapi

import (
	"klynx/internal/services/memsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// DeleteMember godoc
// @Summary Delete a member from a group
// @Description Remove a member from a specified group by UserID and GroupID
// @Tags Members
// @Accept  json
// @Produce  json
// @Param userID query string true "User ID of the member to delete"
// @Param groupID query string true "Group ID from which to delete the member"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members [delete]
// @Security BearerAuth
func DeleteMember(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/memapi", "memapi.DeleteMember", "memapi", "DeleteMember")
	defer span.End()

	memberId := strings.TrimSpace(c.Params("id"))
	if memberId == "" {
		log.Error().Str("memberId", memberId).Msg("❌ Failed to delete member")
		return httputil.FailBadRequest(c, "MISSING_FIELDS", "Member ID is required")
	}

	err := memsvc.DeleteMember(ctx, memberId)
	if err != nil {
		log.Error().Err(err).Str("memberId", memberId).Msg("❌ Failed to delete member")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_DELETE_MEMBER")
	}

	log.Info().Str("memberId", memberId).Msg("✅ Member deleted")
	return gmod.SendMessageOK(c, "MEMBER_DELETED", "Member deleted successfully")
}
