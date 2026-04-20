// controllers/bindingapi/binding.go
package bindingapi

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/bindingsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// bindingSvcI is the service subset used by BindingController.
// *bindingsvc.BindingService satisfies this interface.
type bindingSvcI interface {
	Create(ctx context.Context, input bindingsvc.CreateBindingInput) (*ingestmod.TemplateDeliveryBinding, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error)
	GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.TemplateDeliveryBinding, error)
	Update(ctx context.Context, input bindingsvc.UpdateBindingInput) (*ingestmod.TemplateDeliveryBinding, error)
	Delete(ctx context.Context, workspaceId, id string) error
}

// BindingController handles workspace-scoped TemplateDeliveryBinding CRUD.
// Mounted at /workspaces/:workspaceId/delivery-bindings
type BindingController struct {
	service bindingSvcI
}

func NewBindingController(svc bindingSvcI) *BindingController {
	if svc == nil {
		panic("BindingController: service required")
	}
	return &BindingController{service: svc}
}

// ============================================================
// Request bodies
// ============================================================

type createBindingRequest struct {
	TemplateID        string         `json:"templateId"`
	TargetID          string         `json:"targetId"`
	DispatchStage     string         `json:"dispatchStage"`               // "normalize" | "realtime"
	MatchFields       map[string]any `json:"matchFields,omitempty"`
	MessageTemplateID string         `json:"messageTemplateId,omitempty"`
	Enabled           *bool          `json:"enabled,omitempty"`
}

type updateBindingRequest struct {
	DispatchStage     *string        `json:"dispatchStage,omitempty"`
	MatchFields       map[string]any `json:"matchFields,omitempty"`
	MessageTemplateID *string        `json:"messageTemplateId,omitempty"`
	Enabled           *bool          `json:"enabled,omitempty"`
}

// ============================================================
// Create  POST /workspaces/:workspaceId/delivery-bindings
// ============================================================

// Create godoc
// @Summary      Create a delivery binding
// @Description  Creates a TemplateDeliveryBinding linking an ingest template to a delivery target.
// @Tags         Delivery Bindings
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string                true  "Workspace ID"
// @Param        body         body  createBindingRequest  true  "Binding input"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-bindings [post]
// @Security     BearerAuth
func (ctrl *BindingController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.bindingapi", "BindingController.Create", "bindingapi", "Create")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	var body createBindingRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	if body.TemplateID == "" || body.TargetID == "" || body.DispatchStage == "" {
		return httputil.FailBadRequest(c, "templateId, targetId, dispatchStage required")
	}
	if body.DispatchStage != ingestmod.DispatchStageNormalize && body.DispatchStage != ingestmod.DispatchStageRealtime {
		return httputil.FailBadRequest(c, "dispatchStage must be 'normalize' or 'realtime'")
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	result, err := ctrl.service.Create(ctx, bindingsvc.CreateBindingInput{
		WorkspaceID:       workspaceId,
		TemplateID:        body.TemplateID,
		TargetID:          body.TargetID,
		DispatchStage:     body.DispatchStage,
		MatchFields:       body.MatchFields,
		MessageTemplateID: body.MessageTemplateID,
		Enabled:           enabled,
	})
	if err != nil {
		log.Error().Err(err).Msg("create binding failed")
		status, code := bindingsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Created(c, result, "delivery binding created")
}

// ============================================================
// List  GET /workspaces/:workspaceId/delivery-bindings
// ============================================================

// List godoc
// @Summary      List delivery bindings
// @Description  Returns paginated delivery bindings for the workspace.
// @Tags         Delivery Bindings
// @Produce      json
// @Param        workspaceId  path   string  true   "Workspace ID"
// @Param        page         query  int     false  "Page number (default 1)"
// @Param        perPage      query  int     false  "Items per page (default 20)"
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-bindings [get]
// @Security     BearerAuth
func (ctrl *BindingController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.bindingapi", "BindingController.List", "bindingapi", "List")
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
// GetOne  GET /workspaces/:workspaceId/delivery-bindings/:id
// ============================================================

// GetOne godoc
// @Summary      Get a delivery binding
// @Description  Returns a single delivery binding by ID.
// @Tags         Delivery Bindings
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Binding ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-bindings/{id} [get]
// @Security     BearerAuth
func (ctrl *BindingController) GetOne(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.bindingapi", "BindingController.GetOne", "bindingapi", "GetOne")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	item, err := ctrl.service.GetOne(ctx, workspaceId, id)
	if err != nil {
		status, code := bindingsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, item)
}

// ============================================================
// Update  PATCH /workspaces/:workspaceId/delivery-bindings/:id
// ============================================================

// Update godoc
// @Summary      Update a delivery binding
// @Description  Patches a delivery binding by ID.
// @Tags         Delivery Bindings
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string               true  "Workspace ID"
// @Param        id           path  string               true  "Binding ID"
// @Param        body         body  updateBindingRequest true  "Fields to update"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-bindings/{id} [patch]
// @Security     BearerAuth
func (ctrl *BindingController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.bindingapi", "BindingController.Update", "bindingapi", "Update")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	var body updateBindingRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	result, err := ctrl.service.Update(ctx, bindingsvc.UpdateBindingInput{
		WorkspaceID:       workspaceId,
		ID:                id,
		DispatchStage:     body.DispatchStage,
		MatchFields:       body.MatchFields,
		MessageTemplateID: body.MessageTemplateID,
		Enabled:           body.Enabled,
	})
	if err != nil {
		log.Error().Err(err).Msg("update binding failed")
		status, code := bindingsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result)
}

// ============================================================
// Delete  DELETE /workspaces/:workspaceId/delivery-bindings/:id
// ============================================================

// Delete godoc
// @Summary      Delete a delivery binding
// @Description  Removes a delivery binding by ID.
// @Tags         Delivery Bindings
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Binding ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-bindings/{id} [delete]
// @Security     BearerAuth
func (ctrl *BindingController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.bindingapi", "BindingController.Delete", "bindingapi", "Delete")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	if err := ctrl.service.Delete(ctx, workspaceId, id); err != nil {
		log.Error().Err(err).Msg("delete binding failed")
		status, code := bindingsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.MessageOK(c, "delivery binding deleted")
}
