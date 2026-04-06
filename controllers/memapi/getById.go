package memapi

import (
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/memsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetMemberByID godoc
// @Summary Get member by ID
// @Description Return member detail by ID
// @Tags members
// @Produce  json
// @Param id path string true "member ID"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members/{id} [get]
// @Security BearerAuth
func MemberGetByID(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.memapi", "memapi.MemberGetByID", "memapi", "MemberGetByID")
	defer end()

	memberId := strings.TrimSpace(c.Params("Id"))
	if memberId == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid memberId")
	}

	member, err := memsvc.MemberGetByID(ctx, memberId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "[func MemberGetByID] member not found")
		}
		log.Error().Err(err).Str("memberId", memberId).Msg("❌ MemberGetById failed")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_MEMBER")
	}

	return httputil.Ok(c, member, "member retrieved successfully")
}
