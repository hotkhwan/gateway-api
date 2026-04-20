// controllers/aiconfigdraftapi/draft.go
package aiconfigdraftapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// fromPromptRequest is the request body for the from-prompt endpoint.
type fromPromptRequest struct {
	Prompt string `json:"prompt"`
}

// FromPrompt godoc
// @Summary      Create config draft from prompt
// @Description  Parses a natural-language prompt and creates a new AI config draft.
// @Tags         ConfigDrafts
// @Accept       json
// @Produce      json
// @Param        body  body  fromPromptRequest  true  "Prompt input"
// @Success      201   {object}  gmod.SuccessDataResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      401   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /ingest/config-drafts/from-prompt [post]
// @Security     BearerAuth
func (ctrl *ConfigDraftController) FromPrompt(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aiconfigdraftapi", "ConfigDraftController.FromPrompt", "aiconfigdraftapi", "FromPrompt")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)

	var req fromPromptRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("workspaceId", workspaceId).Msg("invalid request body for from-prompt")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	if req.Prompt == "" {
		return httputil.FailBadRequest(c, "prompt is required")
	}

	draft, err := ctrl.svc.FromPrompt(ctx, workspaceId, userId, req.Prompt)
	if err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("failed to create config draft from prompt")
		return httputil.FailInternal(c, "failed to create config draft")
	}

	return httputil.Created(c, draft, "config draft created")
}
