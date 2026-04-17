// controllers/aiconfigdraftapi/refine.go
package aiconfigdraftapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// Refine godoc
// @Summary      Refine a config draft
// @Description  Applies user answers to missing field hints and updates the draft status.
// @Tags         ConfigDrafts
// @Accept       json
// @Produce      json
// @Param        draftId  path  string                           true  "Draft ID"
// @Param        body     body  aiconfigdraftsvc.RefineRequest  true  "Answers to missing fields"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/config-drafts/{draftId}/refine [post]
// @Security     BearerAuth
func (ctrl *ConfigDraftController) Refine(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aiconfigdraftapi", "ConfigDraftController.Refine", "aiconfigdraftapi", "Refine")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	draftId := c.Params("draftId")

	var req aiconfigdraftsvc.RefineRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("draftId", draftId).Msg("invalid request body for refine")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	draft, err := ctrl.svc.Refine(ctx, workspaceId, draftId, req)
	if err != nil {
		if errors.Is(err, aiconfigdraftsvc.ErrDraftNotFound) {
			log.Warn().Str("draftId", draftId).Msg("draft not found for refine")
			return httputil.FailNotFound(c, "draft not found")
		}
		log.Error().Err(err).Str("draftId", draftId).Msg("failed to refine config draft")
		return httputil.FailInternal(c, "failed to refine config draft")
	}

	return httputil.Ok(c, draft)
}
