// controllers/aimappingapi/suggest.go
package aimappingapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/aimappingsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// AISuggestRequest is the request body for the AI suggest endpoint.
type AISuggestRequest struct {
	SourceFamily     string                `json:"sourceFamily"`
	SamplePayload    map[string]any        `json:"samplePayload"`
	ExistingMappings []ingestmod.FieldMapping `json:"existingMappings,omitempty"`
}

// AISuggest godoc
// @Summary      AI-suggest field mappings
// @Tags         MappingTemplates
// @Accept       json
// @Produce      json
// @Param        body  body  AISuggestRequest  true  "AI suggest input"
// @Success      200   {object}  gmod.SuccessDataResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      401   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /ingest/mappingTemplates/ai-suggest [post]
// @Security     BearerAuth
func (ctrl *AIMappingController) AISuggest(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aimappingapi", "AIMappingController.AISuggest", "aimappingapi", "AISuggest")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	var req AISuggestRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("workspaceId", workspaceId).Msg("invalid request body for ai-suggest")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	if req.SourceFamily == "" {
		return httputil.FailBadRequest(c, "sourceFamily is required")
	}
	if len(req.SamplePayload) == 0 {
		return httputil.FailBadRequest(c, "samplePayload is required")
	}

	input := aimappingsvc.AISuggestInput{
		OrgID:            orgId,
		WorkspaceID:      workspaceId,
		UserID:           userId,
		SourceFamily:     req.SourceFamily,
		SamplePayload:    req.SamplePayload,
		ExistingMappings: req.ExistingMappings,
	}

	result, err := ctrl.svc.Suggest(ctx, input)
	if err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("ai suggest failed")
		return httputil.FailInternal(c, "ai suggest failed")
	}

	return httputil.Ok(c, result)
}
