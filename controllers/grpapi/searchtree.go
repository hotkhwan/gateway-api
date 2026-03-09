// controllers/grpapi/searchtree.go
package grpapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/grpsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// SearchGroupsTree godoc
// @Summary      Search group tree by name
// @Description  Returns flat list of matching group nodes for tree highlight
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Param        name query string true "Name to search"
// @Success      200 {object} grpmod.GroupTreeResponse
// @Failure      400 {object} gmod.BadRequestResponse
// @Failure      500 {object} gmod.InternalErrorResponse
// @Router       /groups/search [get]
// @Security     BearerAuth
func SearchGroupsTree(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.grpapi", "group.SearchGroupsTree", "grpapi", "SearchGroupsTree")
	defer end()

	search := c.Query("name")
	if search == "" {
		return httputil.FailBadRequest(c, "MISSING_NAME_QUERY", "name query parameter is required")
	}

	log.Info().Str("search", search).Msg("🔍 Searching group tree")

	details, err := grpsvc.SearchTreeByName(ctx, search)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to search group tree")
		return httputil.FailInternal(c, "GROUP_SEARCH_FAILED", err.Error())
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "group tree searched successfully",
		"status":  true,
		"details": details,
	})
}
