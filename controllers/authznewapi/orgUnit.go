// controllers/authznewapi/orgUnit.go
package authznewapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"go.opentelemetry.io/otel"
)

type OrgUnitController struct {
	service *authzsvc.OrgUnitService
}

func NewOrgUnitController(svc *authzsvc.OrgUnitService) *OrgUnitController {
	if svc == nil {
		panic("orgUnitService required")
	}
	return &OrgUnitController{service: svc}
}

type CreateOrgUnitBody struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ParentId    *string `json:"parentId"`
}

type UpdateOrgUnitBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Create godoc
// @Summary Create org unit
// @Tags OrgUnit
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateOrgUnitBody true "payload"
// @Success 200 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/units [post]
func (ctrl *OrgUnitController) Create(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authznewapi")
	ctx, span := tracer.Start(ctx, "OrgUnit.Create")
	defer span.End()

	var body CreateOrgUnitBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "name required",
			Status:  false,
		})
	}

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	unitId, err := ctrl.service.CreateOrgUnit(
		ctx,
		tenantId,
		orgId,
		body.Name,
		body.Description,
		body.ParentId,
		userId,
	)

	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageCreateResponse{
		Code:    gmod.CodeCreated,
		Status:  true,
		Message: "orgUnit created",
		ID:      unitId,
	})
}

// Tree godoc
// @Summary Get org unit tree
// @Tags OrgUnit
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/orgs/units/tree [get]
func (ctrl *OrgUnitController) Tree(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authznewapi")
	ctx, span := tracer.Start(ctx, "OrgUnit.Tree")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	tree, err := ctrl.service.GetOrgUnitTree(
		ctx,
		tenantId,
		orgId,
	)

	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"status":  true,
		"details": tree,
	})
}

// Update godoc
// @Summary Update org unit
// @Tags OrgUnit
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "unitId"
// @Param body body UpdateOrgUnitBody true "payload"
// @Router /api/v1/orgs/units/{id} [patch]
func (ctrl *OrgUnitController) Update(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authznewapi")
	ctx, span := tracer.Start(ctx, "OrgUnit.Update")
	defer span.End()

	unitId := strings.TrimSpace(c.Params("id"))

	var body UpdateOrgUnitBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	err := ctrl.service.UpdateOrgUnit(
		ctx,
		tenantId,
		orgId,
		unitId,
		body.Name,
		body.Description,
		userId,
	)

	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "updated",
	})
}

// Delete godoc
// @Summary Delete org unit
// @Tags OrgUnit
// @Security BearerAuth
// @Produce json
// @Param id path string true "unitId"
// @Router /api/v1/orgs/units/{id} [delete]
func (ctrl *OrgUnitController) Delete(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("authznewapi")
	ctx, span := tracer.Start(ctx, "OrgUnit.Delete")
	defer span.End()

	unitId := strings.TrimSpace(c.Params("id"))

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	err := ctrl.service.DeleteOrgUnit(
		ctx,
		tenantId,
		orgId,
		unitId,
	)

	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code:    code,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Status:  true,
		Message: "deleted",
	})
}
