// controllers/aiconfigdraftapi/save.go
package aiconfigdraftapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// Save godoc
// @Summary      Save a config draft
// @Description  Marks the draft status as ready and persists the change.
// @Tags         ConfigDrafts
// @Produce      json
// @Param        draftId  path  string  true  "Draft ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/config-drafts/{draftId}/save [post]
// @Security     BearerAuth
func (ctrl *ConfigDraftController) Save(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aiconfigdraftapi", "ConfigDraftController.Save", "aiconfigdraftapi", "Save")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	draftId := c.Params("draftId")

	draft, err := ctrl.svc.Save(ctx, workspaceId, draftId, userId)
	if err != nil {
		if errors.Is(err, aiconfigdraftsvc.ErrDraftNotFound) {
			log.Warn().Str("draftId", draftId).Msg("draft not found for save")
			return httputil.FailNotFound(c, "draft not found")
		}
		log.Error().Err(err).Str("draftId", draftId).Msg("failed to save config draft")
		return httputil.FailInternal(c, "failed to save config draft")
	}

	return httputil.Ok(c, draft)
}
