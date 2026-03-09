// controllers/grpapi/create.go
package grpapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/models/grpmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// CreateGroup godoc
// @Summary Create a new group
// @Description Create a group, optionally as a child of another group
// @Tags Groups
// @Accept  json
// @Produce  json
// @Param group body grpmod.GroupRequest true "Group data"
// @Success 201 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /groups [post]
// @Security BearerAuth
func CreateGroup(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.grpapi", "group.CreateGroup", "grpapi", "CreateGroup")
	defer end()

	var req grpmod.GroupRequest
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ Invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	if req.Name == "" {
		return httputil.FailBadRequest(c, "MISSING_NAME", "Group name is required")
	}

	err := grpsvc.CreateGroup(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to create group")
		return httputil.FailInternal(c, "CREATE_FAILED", "Failed to create group")
	}

	log.Info().Str("name", req.Name).Msg("✅ Group created")
	return httputil.Created(c, nil, "Group created successfully")
}
