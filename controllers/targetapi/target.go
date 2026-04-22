// controllers/targetapi/target.go
package targetapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type TargetController struct {
	service *targetsvc.TargetService
}

func NewTargetController(svc *targetsvc.TargetService) *TargetController {
	if svc == nil {
		panic("TargetController: service required")
	}
	return &TargetController{service: svc}
}

// ============================================================
// Request bodies
// ============================================================

type CreateTargetRequest struct {
	Name    string                `json:"name"`
	Type    string                `json:"type"`    // webhook|line|telegram|discord
	Enabled *bool                 `json:"enabled"` // default true
	Config  authzmod.TargetConfig `json:"config"`
}

type UpdateTargetRequest struct {
	Name    *string                `json:"name"`
	Enabled *bool                  `json:"enabled"`
	Config  *authzmod.TargetConfig `json:"config"`
}

// ============================================================
// Create  POST /api/v1/targets
// ============================================================

func (ctrl *TargetController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.targetapi", "TargetController.Create", "targetapi", "Create")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	role, _ := c.Locals("role").(string)

	if tenantId == "" || orgId == "" || userId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	var body CreateTargetRequest
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
		TenantId:        tenantId,
		WorkspaceId:     orgId,
		UserId:          userId,
		Name:            body.Name,
		Type:            body.Type,
		Enabled:         enabled,
		Config:          body.Config,
		IsPlatformAdmin: role == "administrator",
	})
	if err != nil {
		log.Error().Err(err).Msg("create target failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Created(c, result, "delivery target created")
}

// ============================================================
// List  GET /api/v1/targets
// ============================================================

func (ctrl *TargetController) List(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.targetapi", "TargetController.List", "targetapi", "List")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)

	if tenantId == "" || orgId == "" {
		return httputil.FailUnauthorized(c, "unauthorized")
	}

	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPage", 20)
	search := strings.TrimSpace(c.Query("search"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	items, total, err := ctrl.service.List(ctx, targetsvc.ListTargetInput{
		TenantId:    tenantId,
		WorkspaceId: orgId,
		Search:      search,
		Page:        page,
		PerPage:     perPage,
		SortField:   sortField,
		SortOrder:   sortOrder,
	})
	if err != nil {
		log.Error().Err(err).Msg("list targets failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"status":  true,
		"message": "ok",
		"details": items,
		"pagination": gmod.Pagination{
			Page:         page,
			PerPage:     perPage,
			TotalRecords: int(total),
			TotalPages:   totalPages,
			SortField:    sortField,
			SortOrder:    sortOrder,
		},
	})
}

// ============================================================
// GetOne  GET /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) GetOne(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.targetapi", "TargetController.GetOne", "targetapi", "GetOne")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	role, _ := c.Locals("role").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return httputil.FailBadRequest(c, "missing params")
	}

	result, err := ctrl.service.GetOne(ctx, tenantId, orgId, userId, targetId, role == "administrator")
	if err != nil {
		log.Error().Err(err).Msg("get target failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result)
}

// ============================================================
// Update  PATCH /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.targetapi", "TargetController.Update", "targetapi", "Update")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	role, _ := c.Locals("role").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return httputil.FailBadRequest(c, "missing params")
	}

	var body UpdateTargetRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	result, err := ctrl.service.Update(ctx, targetsvc.UpdateTargetInput{
		TenantId:        tenantId,
		WorkspaceId:     orgId,
		TargetId:        targetId,
		UserId:          userId,
		Name:            body.Name,
		Enabled:         body.Enabled,
		Config:          body.Config,
		IsPlatformAdmin: role == "administrator",
	})
	if err != nil {
		log.Error().Err(err).Msg("update target failed")
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return httputil.Ok(c, result, "target updated")
}

// ============================================================
// Delete  DELETE /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.targetapi", "TargetController.Delete", "targetapi", "Delete")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeWorkspace").(string)
	userId, _ := c.Locals("userId").(string)
	role, _ := c.Locals("role").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return httputil.FailBadRequest(c, "missing params")
	}

	if err := ctrl.service.Delete(ctx, tenantId, orgId, userId, targetId, role == "administrator"); err != nil {
		log.Error().Err(err).Msg("delete target failed")
		status, code := targetsvc.MapSvcError(err)
		resp := gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false}
		var inUse *targetsvc.TargetInUseError
		if errors.As(err, &inUse) {
			resp.Details = fiber.Map{"templates": inUse.Templates}
		}
		return c.Status(status).JSON(resp)
	}

	return httputil.MessageOK(c, "target deleted")
}
