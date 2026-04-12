// controllers/msgtmplapi/msgTemplate.go
package msgtmplapi

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/msgtmplsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// msgTmplSvcI is the service subset used by MsgTemplateController.
// *msgtmplsvc.MsgTemplateService satisfies this interface.
type msgTmplSvcI interface {
	Create(ctx context.Context, input msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error)
	GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error)
	Update(ctx context.Context, input msgtmplsvc.UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error)
	Delete(ctx context.Context, workspaceId, id string) error
}

// MsgTemplateController handles workspace-scoped WorkspaceMessageTemplate CRUD.
// Mounted at /workspaces/:workspaceId/message-templates
type MsgTemplateController struct {
	service msgTmplSvcI
}

func NewMsgTemplateController(svc msgTmplSvcI) *MsgTemplateController {
	if svc == nil {
		panic("MsgTemplateController: service required")
	}
	return &MsgTemplateController{service: svc}
}

// ============================================================
// Request bodies
// ============================================================

type createMsgTemplateRequest struct {
	Name    string `json:"name"`
	Channel string `json:"channel,omitempty"` // line|webhook|telegram|discord
	Body    string `json:"body,omitempty"`
	Locale  string `json:"locale,omitempty"`
}

type updateMsgTemplateRequest struct {
	Name    *string `json:"name,omitempty"`
	Channel *string `json:"channel,omitempty"`
	Body    *string `json:"body,omitempty"`
	Locale  *string `json:"locale,omitempty"`
}

// ============================================================
// Create  POST /workspaces/:workspaceId/message-templates
// ============================================================

// Create godoc
// @Summary      Create a message template
// @Description  Creates a WorkspaceMessageTemplate for channel-based delivery notifications.
// @Tags         Message Templates
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string                    true  "Workspace ID"
// @Param        body         body  createMsgTemplateRequest  true  "Template input"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      409  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/message-templates [post]
// @Security     BearerAuth
func (ctrl *MsgTemplateController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.msgtmplapi", "MsgTemplateController.Create", "msgtmplapi", "Create")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	var body createMsgTemplateRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name required")
	}

	result, err := ctrl.service.Create(ctx, msgtmplsvc.CreateMsgTemplateInput{
		WorkspaceID: workspaceId,
		Name:        body.Name,
		Channel:     strings.TrimSpace(body.Channel),
		Body:        body.Body,
		Locale:      strings.TrimSpace(body.Locale),
	})
	if err != nil {
		log.Error().Err(err).Msg("create message template failed")
		status, code := msgtmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Created(c, result, "message template created")
}

// ============================================================
// List  GET /workspaces/:workspaceId/message-templates
// ============================================================

// List godoc
// @Summary      List message templates
// @Description  Returns paginated message templates for the workspace.
// @Tags         Message Templates
// @Produce      json
// @Param        workspaceId  path   string  true   "Workspace ID"
// @Param        page         query  int     false  "Page number (default 1)"
// @Param        perPage      query  int     false  "Items per page (default 20)"
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/message-templates [get]
// @Security     BearerAuth
func (ctrl *MsgTemplateController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.msgtmplapi", "MsgTemplateController.List", "msgtmplapi", "List")
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
// GetOne  GET /workspaces/:workspaceId/message-templates/:id
// ============================================================

// GetOne godoc
// @Summary      Get a message template
// @Description  Returns a single message template by ID.
// @Tags         Message Templates
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/message-templates/{id} [get]
// @Security     BearerAuth
func (ctrl *MsgTemplateController) GetOne(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.msgtmplapi", "MsgTemplateController.GetOne", "msgtmplapi", "GetOne")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	item, err := ctrl.service.GetOne(ctx, workspaceId, id)
	if err != nil {
		status, code := msgtmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, item)
}

// ============================================================
// Update  PATCH /workspaces/:workspaceId/message-templates/:id
// ============================================================

// Update godoc
// @Summary      Update a message template
// @Description  Patches a message template by ID.
// @Tags         Message Templates
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string                    true  "Workspace ID"
// @Param        id           path  string                    true  "Template ID"
// @Param        body         body  updateMsgTemplateRequest  true  "Fields to update"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/message-templates/{id} [patch]
// @Security     BearerAuth
func (ctrl *MsgTemplateController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.msgtmplapi", "MsgTemplateController.Update", "msgtmplapi", "Update")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	var body updateMsgTemplateRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	result, err := ctrl.service.Update(ctx, msgtmplsvc.UpdateMsgTemplateInput{
		WorkspaceID: workspaceId,
		ID:          id,
		Name:        body.Name,
		Channel:     body.Channel,
		Body:        body.Body,
		Locale:      body.Locale,
	})
	if err != nil {
		log.Error().Err(err).Msg("update message template failed")
		status, code := msgtmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result)
}

// ============================================================
// Delete  DELETE /workspaces/:workspaceId/message-templates/:id
// ============================================================

// Delete godoc
// @Summary      Delete a message template
// @Description  Removes a message template by ID.
// @Tags         Message Templates
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/message-templates/{id} [delete]
// @Security     BearerAuth
func (ctrl *MsgTemplateController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.msgtmplapi", "MsgTemplateController.Delete", "msgtmplapi", "Delete")
	defer end()

	workspaceId, _ := c.Locals("activeWorkspace").(string)
	id := strings.TrimSpace(c.Params("id"))

	if workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	if err := ctrl.service.Delete(ctx, workspaceId, id); err != nil {
		log.Error().Err(err).Msg("delete message template failed")
		status, code := msgtmplsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.MessageOK(c, "message template deleted")
}
