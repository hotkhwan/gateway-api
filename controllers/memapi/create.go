package memapi

import (
	"klynx/internal/services/memsvc"
	"klynx/models/gmod"
	"klynx/models/memmod"
	"klynx/utils/authutil"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// CreateMember godoc
// @Summary Create a new member in a group
// @Description Add a new member to a specified group
// @Tags Members
// @Accept  json
// @Produce  json
// @Param member body memmod.Member true "Member data"
// @Success 201 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /members [post]
// @Security BearerAuth
func CreateMember(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/memapi", "memapi.CreateMember", "memapi", "CreateMember")
	defer span.End()

	var req []memmod.MemberRequest
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ Invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
	}

	validate := []string{}
	for _, r := range req {
		if r.UserID == "" {
			validate = append(validate, "userID")
		}
		if r.GroupID == "" {
			validate = append(validate, "groupID")
		}
	}

	if len(validate) > 0 {
		log.Error().Strs("missingFields", validate).Msg("❌ [MemberCreate] Missing required fields")
		return httputil.FailBadRequest(c, "MISSING_FIELDS", "Missing required fields: "+authutil.JoinFields(validate))
	}

	err := memsvc.CreateMember(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to create member")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_CREATE_MEMBER")
	}
	return gmod.SendCreatedMessage(c, "MEMBER_CREATED", "Member created successfully")
}

func CreateMember2(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/memapi", "memapi.CreateMember2", "memapi", "CreateMember2")
	defer span.End()

	// ดึง groupID จาก URL: /member/:groupID
	groupID := c.Params("groupID")
	if groupID == "" {
		return httputil.FailBadRequest(c, "INVALID_PATH", "groupID is required")
	}

	var req memmod.MemberRequest2
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ Invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
	}

	// ส่ง groupID และ userIds (Slice) ไปทำงาน
	err := memsvc.CreateMember2(ctx, groupID, req.UserIds)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to create member")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_CREATE_MEMBER")
	}

	return gmod.SendCreatedMessage(c, "MEMBER_CREATED", "Group members updated successfully")
}
