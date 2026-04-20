// controllers/ingesttmplapi/ingestTemplate.go
package ingesttmplapi

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingesttmplsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ingestTmplSvcI is the service subset used by IngestTemplateController.
// *ingesttmplsvc.IngestTemplateService satisfies this interface.
type ingestTmplSvcI interface {
	Create(ctx context.Context, input ingesttmplsvc.CreateIngestTemplateInput) (*ingestmod.IngestTemplate, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error)
	GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.IngestTemplate, error)
	Update(ctx context.Context, input ingesttmplsvc.UpdateIngestTemplateInput) (*ingestmod.IngestTemplate, error)
	Delete(ctx context.Context, workspaceId, id string) error
}

// IngestTemplateController handles workspace-scoped IngestTemplate CRUD.
// Mounted at /workspaces/:workspaceId/ingest-templates
type IngestTemplateController struct {
	service ingestTmplSvcI
}

func NewIngestTemplateController(svc ingestTmplSvcI) *IngestTemplateController {
	if svc == nil {
		panic("IngestTemplateController: service required")
	}
	return &IngestTemplateController{service: svc}
}

// ============================================================
// Request bodies
// ============================================================

type createIngestTemplateRequest struct {
	Name         string             `json:"name"`
	SourceFamily string             `json:"sourceFamily"`
	MatchRules   []ingestmod.MatchRule `json:"matchRules,omitempty"`
	FieldMapping map[string]any     `json:"fieldMapping,omitempty"`
	Enabled      *bool              `json:"enabled,omitempty"`
}

type updateIngestTemplateRequest struct {
	Name         *string            `json:"name,omitempty"`
	SourceFamily *string            `json:"sourceFamily,omitempty"`
	MatchRules   []ingestmod.MatchRule `json:"matchRules,omitempty"`
	FieldMapping map[string]any     `json:"fieldMapping,omitempty"`
	Enabled      *bool              `json:"enabled,omitempty"`
}

// ============================================================
// Create  POST /workspaces/:workspaceId/ingest-templates
// ============================================================

// Create godoc
// @Summary      Create an ingest template
// @Description  Creates an IngestTemplate that classifies raw events for the workspace.
// @Tags         Ingest Templates
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string                       true  "Workspace ID"
// @Param        body         body  createIngestTemplateRequest  true  "Template input"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      409  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/ingest-templates [post]
// @Security     BearerAuth
func (ctrl *IngestTemplateController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingesttmplapi", "IngestTemplateController.Create", "ingesttmplapi", "Create")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	var body createIngestTemplateRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.SourceFamily = strings.TrimSpace(body.SourceFamily)
	if body.Name == "" || body.SourceFamily == "" {
		return httputil.FailBadRequest(c, "name and sourceFamily required")
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	result, err := ctrl.service.Create(ctx, ingesttmplsvc.CreateIngestTemplateInput{
		WorkspaceID:  workspaceId,
		Name:         body.Name,
		SourceFamily: body.SourceFamily,
		MatchRules:   body.MatchRules,
		FieldMapping: body.FieldMapping,
		Enabled:      enabled,
	})
	if err != nil {
		log.Error().Err(err).Msg("create ingest template failed")
		status, code := ingesttmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Created(c, result, "ingest template created")
}

// ============================================================
// List  GET /workspaces/:workspaceId/ingest-templates
// ============================================================

// List godoc
// @Summary      List ingest templates
// @Description  Returns paginated ingest templates for the workspace.
// @Tags         Ingest Templates
// @Produce      json
// @Param        workspaceId  path   string  true   "Workspace ID"
// @Param        page         query  int     false  "Page number (default 1)"
// @Param        perPage      query  int     false  "Items per page (default 20)"
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/ingest-templates [get]
// @Security     BearerAuth
func (ctrl *IngestTemplateController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.ingesttmplapi", "IngestTemplateController.List", "ingesttmplapi", "List")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPage", 20)

	items, pag, err := ctrl.service.List(ctx, workspaceId, page, perPage)
	if err != nil {
		return httputil.FailInternal(c, "list failed")
	}

	resp := fiber.Map{
		"code":    "SUCCESS",
		"status":  true,
		"message": "ok",
		"details": fiber.Map{"items": items},
	}
	if pag != nil {
		resp["pagination"] = pag
	}
	return c.JSON(resp)
}

// ============================================================
// GetOne  GET /workspaces/:workspaceId/ingest-templates/:id
// ============================================================

// GetOne godoc
// @Summary      Get an ingest template
// @Description  Returns a single ingest template by ID.
// @Tags         Ingest Templates
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/ingest-templates/{id} [get]
// @Security     BearerAuth
func (ctrl *IngestTemplateController) GetOne(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.ingesttmplapi", "IngestTemplateController.GetOne", "ingesttmplapi", "GetOne")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	item, err := ctrl.service.GetOne(ctx, workspaceId, id)
	if err != nil {
		status, code := ingesttmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, item)
}

// ============================================================
// Update  PATCH /workspaces/:workspaceId/ingest-templates/:id
// ============================================================

// Update godoc
// @Summary      Update an ingest template
// @Description  Patches an ingest template by ID.
// @Tags         Ingest Templates
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string                       true  "Workspace ID"
// @Param        id           path  string                       true  "Template ID"
// @Param        body         body  updateIngestTemplateRequest  true  "Fields to update"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/ingest-templates/{id} [patch]
// @Security     BearerAuth
func (ctrl *IngestTemplateController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingesttmplapi", "IngestTemplateController.Update", "ingesttmplapi", "Update")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	var body updateIngestTemplateRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	result, err := ctrl.service.Update(ctx, ingesttmplsvc.UpdateIngestTemplateInput{
		WorkspaceID:  workspaceId,
		ID:           id,
		Name:         body.Name,
		SourceFamily: body.SourceFamily,
		MatchRules:   body.MatchRules,
		FieldMapping: body.FieldMapping,
		Enabled:      body.Enabled,
	})
	if err != nil {
		log.Error().Err(err).Msg("update ingest template failed")
		status, code := ingesttmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result)
}

// ============================================================
// Delete  DELETE /workspaces/:workspaceId/ingest-templates/:id
// ============================================================

// Delete godoc
// @Summary      Delete an ingest template
// @Description  Removes an ingest template by ID.
// @Tags         Ingest Templates
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/ingest-templates/{id} [delete]
// @Security     BearerAuth
func (ctrl *IngestTemplateController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingesttmplapi", "IngestTemplateController.Delete", "ingesttmplapi", "Delete")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	if err := ctrl.service.Delete(ctx, workspaceId, id); err != nil {
		log.Error().Err(err).Msg("delete ingest template failed")
		status, code := ingesttmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.MessageOK(c, "ingest template deleted")
}
