// controllers/wstargetapi/target.go
package wstargetapi

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// targetSvcI is the service subset used by WsTargetController.
// *targetsvc.TargetService satisfies this interface.
type targetSvcI interface {
	Create(ctx context.Context, input targetsvc.CreateTargetInput) (*authzmod.DeliveryTarget, error)
	List(ctx context.Context, input targetsvc.ListTargetInput) ([]authzmod.DeliveryTarget, int64, error)
	GetOne(ctx context.Context, tenantId, workspaceId, userId, targetId string) (*authzmod.DeliveryTarget, error)
	Update(ctx context.Context, input targetsvc.UpdateTargetInput) (*authzmod.DeliveryTarget, error)
	Delete(ctx context.Context, tenantId, workspaceId, userId, targetId string) error
}

// WsTargetController handles workspace-scoped delivery target CRUD.
// Mounted at /workspaces/:workspaceId/delivery-targets
// Extends the base TargetService with mode=klynx support.
type WsTargetController struct {
	service targetSvcI
}

func NewWsTargetController(svc targetSvcI) *WsTargetController {
	if svc == nil {
		panic("WsTargetController: service required")
	}
	return &WsTargetController{service: svc}
}

// ============================================================
// Request bodies
// ============================================================

type createTargetRequest struct {
	Name    string                `json:"name"`
	Type    string                `json:"type"`             // webhook|line|telegram|discord
	Mode    string                `json:"mode,omitempty"`   // "klynx" for system routing marker only
	Enabled *bool                 `json:"enabled,omitempty"`
	Config  authzmod.TargetConfig `json:"config"`
}

type updateTargetRequest struct {
	Name    *string                `json:"name,omitempty"`
	Enabled *bool                  `json:"enabled,omitempty"`
	Config  *authzmod.TargetConfig `json:"config,omitempty"`
}

// ============================================================
// Create  POST /workspaces/:workspaceId/delivery-targets
// ============================================================

// Create godoc
// @Summary      Create a delivery target
// @Description  Creates a delivery target for the workspace. Use mode=klynx for EventBridge routing (appliance only).
// @Tags         Delivery Targets
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string               true  "Workspace ID"
// @Param        body         body  createTargetRequest  true  "Target input"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      409  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-targets [post]
// @Security     BearerAuth
func (ctrl *WsTargetController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.wstargetapi", "WsTargetController.Create", "wstargetapi", "Create")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)

	if tenantId == "" || workspaceId == "" || userId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	var body createTargetRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name required")
	}
	if body.Type == "" {
		return httputil.FailBadRequest(c, "type required (webhook|line|telegram|discord)")
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	result, err := ctrl.service.Create(ctx, targetsvc.CreateTargetInput{
		TenantId:    tenantId,
		WorkspaceId: workspaceId,
		UserId:      userId,
		Name:        body.Name,
		Type:        body.Type,
		Mode:        strings.TrimSpace(body.Mode),
		Enabled:     enabled,
		Config:      body.Config,
	})
	if err != nil {
		log.Error().Err(err).Msg("create delivery target failed")
		if errors.Is(err, targetsvc.ErrKlynxModeWithURL) ||
			errors.Is(err, targetsvc.ErrKlynxModeWithHMAC) ||
			errors.Is(err, targetsvc.ErrKlynxModeInSaasPublic) {
			return httputil.FailBadRequest(c, err.Error())
		}
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Created(c, result, "delivery target created")
}

// ============================================================
// List  GET /workspaces/:workspaceId/delivery-targets
// ============================================================

// List godoc
// @Summary      List delivery targets
// @Description  Returns paginated delivery targets for the workspace.
// @Tags         Delivery Targets
// @Produce      json
// @Param        workspaceId  path   string  true   "Workspace ID"
// @Param        page         query  int     false  "Page number (default 1)"
// @Param        perPage      query  int     false  "Items per page (default 20)"
// @Param        search       query  string  false  "Name search"
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-targets [get]
// @Security     BearerAuth
func (ctrl *WsTargetController) List(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.wstargetapi", "WsTargetController.List", "wstargetapi", "List")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)

	if tenantId == "" || workspaceId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPage", 20)
	search := strings.TrimSpace(c.Query("search"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	items, total, err := ctrl.service.List(ctx, targetsvc.ListTargetInput{
		TenantId:    tenantId,
		WorkspaceId: workspaceId,
		Search:      search,
		Page:        page,
		PerPage:     perPage,
		SortField:   sortField,
		SortOrder:   sortOrder,
	})
	if err != nil {
		return httputil.FailInternal(c, "list failed")
	}

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"code":    "SUCCESS",
		"status":  true,
		"message": "ok",
		"details": fiber.Map{"items": items},
		"pagination": gmod.Pagination{
			Page:         page,
			PerPages:     perPage,
			TotalRecords: int(total),
			TotalPages:   totalPages,
			SortField:    sortField,
			SortOrder:    sortOrder,
		},
	})
}

// ============================================================
// GetOne  GET /workspaces/:workspaceId/delivery-targets/:id
// ============================================================

// GetOne godoc
// @Summary      Get a delivery target
// @Description  Returns a single delivery target by ID.
// @Tags         Delivery Targets
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Target ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-targets/{id} [get]
// @Security     BearerAuth
func (ctrl *WsTargetController) GetOne(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.wstargetapi", "WsTargetController.GetOne", "wstargetapi", "GetOne")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	id := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	item, err := ctrl.service.GetOne(ctx, tenantId, workspaceId, userId, id)
	if err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, item)
}

// ============================================================
// Update  PATCH /workspaces/:workspaceId/delivery-targets/:id
// ============================================================

// Update godoc
// @Summary      Update a delivery target
// @Description  Patches a delivery target by ID.
// @Tags         Delivery Targets
// @Accept       json
// @Produce      json
// @Param        workspaceId  path  string               true  "Workspace ID"
// @Param        id           path  string               true  "Target ID"
// @Param        body         body  updateTargetRequest  true  "Fields to update"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ErrorResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-targets/{id} [patch]
// @Security     BearerAuth
func (ctrl *WsTargetController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.wstargetapi", "WsTargetController.Update", "wstargetapi", "Update")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	id := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	var body updateTargetRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	result, err := ctrl.service.Update(ctx, targetsvc.UpdateTargetInput{
		TenantId:    tenantId,
		WorkspaceId: workspaceId,
		TargetId:    id,
		UserId:      userId,
		Name:        body.Name,
		Enabled:     body.Enabled,
		Config:      body.Config,
	})
	if err != nil {
		log.Error().Err(err).Msg("update delivery target failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result)
}

// ============================================================
// Delete  DELETE /workspaces/:workspaceId/delivery-targets/:id
// ============================================================

// Delete godoc
// @Summary      Delete a delivery target
// @Description  Removes a delivery target by ID.
// @Tags         Delivery Targets
// @Produce      json
// @Param        workspaceId  path  string  true  "Workspace ID"
// @Param        id           path  string  true  "Target ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      401  {object}  gmod.ErrorResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Router       /workspaces/{workspaceId}/delivery-targets/{id} [delete]
// @Security     BearerAuth
func (ctrl *WsTargetController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.wstargetapi", "WsTargetController.Delete", "wstargetapi", "Delete")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	id := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || workspaceId == "" || id == "" {
		return httputil.FailBadRequest(c, "invalid params")
	}

	if err := ctrl.service.Delete(ctx, tenantId, workspaceId, userId, id); err != nil {
		log.Error().Err(err).Msg("delete delivery target failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.MessageOK(c, "delivery target deleted")
}
