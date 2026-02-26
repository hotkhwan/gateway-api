// controllers/targetapi/target.go
package targetapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"go.opentelemetry.io/otel"
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

func (ctrl *TargetController) Create(c *fiber.Ctx) error {
	ctx := c.UserContext()
	_, span := otel.Tracer("targetapi").Start(ctx, "Target.Create")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	if tenantId == "" || orgId == "" || userId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{Code: gmod.CodeUnauthorized, Message: "unauthorized", Status: false})
	}

	var body CreateTargetRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name required", Status: false})
	}
	if body.Type == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "type required (webhook|line|telegram|discord)", Status: false})
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	result, err := ctrl.service.Create(ctx, targetsvc.CreateTargetInput{
		TenantId: tenantId,
		OrgId:    orgId,
		UserId:   userId,
		Name:     body.Name,
		Type:     body.Type,
		Enabled:  enabled,
		Config:   body.Config,
	})
	if err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return c.Status(201).JSON(gmod.SuccessDetailResponseWithMSG{
		Code:    gmod.CodeCreated,
		Message: "delivery target created",
		Status:  true,
		Detail:  result,
	})
}

// ============================================================
// List  GET /api/v1/targets
// ============================================================

func (ctrl *TargetController) List(c *fiber.Ctx) error {
	ctx := c.UserContext()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	if tenantId == "" || orgId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{Code: gmod.CodeUnauthorized, Message: "unauthorized", Status: false})
	}

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("perPage", 20)
	search := strings.TrimSpace(c.Query("search"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	items, total, err := ctrl.service.List(ctx, targetsvc.ListTargetInput{
		TenantId:  tenantId,
		OrgId:     orgId,
		Search:    search,
		Page:      page,
		PerPage:   perPage,
		SortField: sortField,
		SortOrder: sortOrder,
	})
	if err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}

	return c.Status(200).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"status":  true,
		"message": "ok",
		"details": items,
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
// GetOne  GET /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) GetOne(c *fiber.Ctx) error {
	ctx := c.UserContext()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "missing params", Status: false})
	}

	result, err := ctrl.service.GetOne(ctx, tenantId, orgId, userId, targetId)
	if err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return c.Status(200).JSON(gmod.SuccessDetailResponseWithMSG{
		Code:    gmod.CodeSuccess,
		Message: "ok",
		Status:  true,
		Detail:  result,
	})
}

// ============================================================
// Update  PATCH /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) Update(c *fiber.Ctx) error {
	ctx := c.UserContext()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "missing params", Status: false})
	}

	var body UpdateTargetRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}

	result, err := ctrl.service.Update(ctx, targetsvc.UpdateTargetInput{
		TenantId: tenantId,
		OrgId:    orgId,
		TargetId: targetId,
		UserId:   userId,
		Name:     body.Name,
		Enabled:  body.Enabled,
		Config:   body.Config,
	})
	if err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return c.Status(200).JSON(gmod.SuccessDetailResponseWithMSG{
		Code:    gmod.CodeSuccess,
		Message: "target updated",
		Status:  true,
		Detail:  result,
	})
}

// ============================================================
// Delete  DELETE /api/v1/targets/:id
// ============================================================

func (ctrl *TargetController) Delete(c *fiber.Ctx) error {
	ctx := c.UserContext()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	targetId := strings.TrimSpace(c.Params("id"))

	if tenantId == "" || orgId == "" || userId == "" || targetId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "missing params", Status: false})
	}

	if err := ctrl.service.Delete(ctx, tenantId, orgId, userId, targetId); err != nil {
		status, code := targetsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{Code: code, Message: err.Error(), Status: false})
	}

	return c.Status(200).JSON(gmod.SuccessResponse{
		Code:    gmod.CodeSuccess,
		Message: "target deleted",
		Status:  true,
	})
}
