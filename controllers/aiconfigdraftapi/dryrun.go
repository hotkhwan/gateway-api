// controllers/aiconfigdraftapi/dryrun.go
package aiconfigdraftapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// dryRunRequest is the request body for the dry-run endpoint.
type dryRunRequest struct {
	SamplePayload map[string]any `json:"samplePayload"`
}

// DryRun godoc
// @Summary      Dry-run a config draft
// @Description  Simulates the draft against a sample payload and returns match/target counts.
// @Tags         ConfigDrafts
// @Accept       json
// @Produce      json
// @Param        draftId  path  string          true  "Draft ID"
// @Param        body     body  dryRunRequest   true  "Sample payload to simulate against"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/config-drafts/{draftId}/dry-run [post]
// @Security     BearerAuth
func (ctrl *ConfigDraftController) DryRun(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aiconfigdraftapi", "ConfigDraftController.DryRun", "aiconfigdraftapi", "DryRun")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	draftId := c.Params("draftId")

	var req dryRunRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("draftId", draftId).Msg("invalid request body for dry-run")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	if len(req.SamplePayload) == 0 {
		return httputil.FailBadRequest(c, "samplePayload is required")
	}

	result, err := ctrl.svc.DryRun(ctx, workspaceId, draftId, req.SamplePayload)
	if err != nil {
		if errors.Is(err, aiconfigdraftsvc.ErrDraftNotFound) {
			log.Warn().Str("draftId", draftId).Msg("draft not found for dry-run")
			return httputil.FailNotFound(c, "draft not found")
		}
		log.Error().Err(err).Str("draftId", draftId).Msg("failed to run dry-run")
		return httputil.FailInternal(c, "failed to run dry-run")
	}

	return httputil.Ok(c, result)
}
