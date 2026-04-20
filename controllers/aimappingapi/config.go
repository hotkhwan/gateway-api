// controllers/aimappingapi/config.go
package aimappingapi

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/aimappingsvc"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// AIConfigResponse is the public view of WorkspaceAIConfig — key is never included.
type AIConfigResponse struct {
	WorkspaceID          string     `json:"workspaceId"`
	Enabled              bool       `json:"enabled"`
	Provider             string     `json:"provider"`
	Model                string     `json:"model"`
	ProviderMode         string     `json:"providerMode"`
	DefaultTimeoutMs     int        `json:"defaultTimeoutMs"`
	MaxInputBytes        int        `json:"maxInputBytes"`
	HasApiKey            bool       `json:"hasApiKey"`
	CreatedBy            string     `json:"createdBy"`
	UpdatedBy            string     `json:"updatedBy"`
	LastValidatedAt      *time.Time `json:"lastValidatedAt"`
	LastValidationStatus string     `json:"lastValidationStatus"`
	LastValidationError  string     `json:"lastValidationError"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// toAIConfigResponse maps a WorkspaceAIConfig to the public response shape.
func toAIConfigResponse(cfg *workspacemod.WorkspaceAIConfig, hasApiKey bool) AIConfigResponse {
	return AIConfigResponse{
		WorkspaceID:          cfg.WorkspaceID,
		Enabled:              cfg.Enabled,
		Provider:             cfg.Provider,
		Model:                cfg.Model,
		ProviderMode:         cfg.ProviderMode,
		DefaultTimeoutMs:     cfg.DefaultTimeoutMs,
		MaxInputBytes:        cfg.MaxInputBytes,
		HasApiKey:            hasApiKey,
		CreatedBy:            cfg.CreatedBy,
		UpdatedBy:            cfg.UpdatedBy,
		LastValidatedAt:      cfg.LastValidatedAt,
		LastValidationStatus: cfg.LastValidationStatus,
		LastValidationError:  cfg.LastValidationError,
		CreatedAt:            cfg.CreatedAt,
		UpdatedAt:            cfg.UpdatedAt,
	}
}

// GetAIConfig godoc
// @Summary      Get workspace AI config
// @Description  Returns workspace AI config. HasApiKey indicates whether a key is stored — the actual key is never returned.
// @Tags         MappingTemplates
// @Produce      json
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/ai-config [get]
// @Security     BearerAuth
func (ctrl *AIMappingController) GetAIConfig(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aimappingapi", "AIMappingController.GetAIConfig", "aimappingapi", "GetAIConfig")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)

	cfg, hasApiKey, err := ctrl.svc.GetConfig(ctx, workspaceId)
	if err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("failed to get ai config")
		return httputil.FailInternal(c, "failed to get ai config")
	}

	if cfg == nil {
		// No config yet — return empty response with hasApiKey: false.
		return httputil.Ok(c, AIConfigResponse{WorkspaceID: workspaceId, HasApiKey: false})
	}

	return httputil.Ok(c, toAIConfigResponse(cfg, hasApiKey))
}

// UpsertAIConfig godoc
// @Summary      Create or update workspace AI config
// @Tags         MappingTemplates
// @Accept       json
// @Produce      json
// @Param        body  body  aimappingsvc.UpsertConfigRequest  true  "AI config input"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      401   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /ingest/ai-config [put]
// @Security     BearerAuth
func (ctrl *AIMappingController) UpsertAIConfig(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aimappingapi", "AIMappingController.UpsertAIConfig", "aimappingapi", "UpsertAIConfig")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)

	var req aimappingsvc.UpsertConfigRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("workspaceId", workspaceId).Msg("invalid request body for upsert ai config")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	if err := ctrl.svc.UpsertConfig(ctx, workspaceId, userId, req); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("failed to upsert ai config")
		return httputil.FailInternal(c, "failed to upsert ai config")
	}

	return httputil.MessageOK(c, "ai config updated")
}

// ClearApiKey godoc
// @Summary      Clear workspace AI API key
// @Tags         MappingTemplates
// @Produce      json
// @Success      204
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/ai-config/key [delete]
// @Security     BearerAuth
func (ctrl *AIMappingController) ClearApiKey(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aimappingapi", "AIMappingController.ClearApiKey", "aimappingapi", "ClearApiKey")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)

	if err := ctrl.svc.ClearApiKey(ctx, workspaceId, userId); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("failed to clear api key")
		return httputil.FailInternal(c, "failed to clear api key")
	}

	return httputil.NoContent(c)
}

// ValidateAIConfigResponse is the response for the validate endpoint.
type ValidateAIConfigResponse struct {
	Status      string     `json:"status"` // "ok" | "fail"
	Error       string     `json:"error,omitempty"`
	ValidatedAt time.Time  `json:"validatedAt"`
}

// ValidateAIConfig godoc
// @Summary      Validate workspace AI provider connection
// @Tags         MappingTemplates
// @Produce      json
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /ingest/ai-config/validate [post]
// @Security     BearerAuth
func (ctrl *AIMappingController) ValidateAIConfig(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "github.com/hotkhwan/gateway-api/aimappingapi", "AIMappingController.ValidateAIConfig", "aimappingapi", "ValidateAIConfig")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)

	resp := ValidateAIConfigResponse{
		ValidatedAt: time.Now().UTC(),
	}

	err := ctrl.svc.ValidateConnection(ctx, workspaceId)
	if err != nil {
		log.Warn().Err(err).Str("workspaceId", workspaceId).Msg("ai config validation failed")
		resp.Status = "fail"
		resp.Error = err.Error()
		return httputil.Ok(c, resp)
	}

	resp.Status = "ok"
	return httputil.Ok(c, resp)
}
