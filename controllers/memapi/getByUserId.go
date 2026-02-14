// controllers/memapi/getByUserId.go
package memapi

import (
	"errors"
	"strings"

	"klynx/internal/services/memsvc"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetMemberByUserID godoc
// @Summary Get member by ID
// @Description Return member detail by ID
// @Tags members
// @Produce  json
// @Param userId path string true "member ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members/{userId} [get]
// @Security BearerAuth
func MemberGetByUserID(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "klynx/devapi", "devapi.memberGetByID", "devapi", "memberGetByID")
	defer end()

	userId := strings.TrimSpace(c.Params("Id"))
	if userId == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid userId")
	}

	member, err := memsvc.GetGroupByUserID(ctx, userId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "member not found")
		}
		log.Error().Err(err).Str("memberId", userId).Msg("❌ MemberGetByUserId failed")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_MEMBER")
	}

	return httputil.Ok(c, member)
}
