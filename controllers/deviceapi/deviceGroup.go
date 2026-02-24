// controllers/deviceapi/deviceGroup.go
package deviceapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// ============================================================
// Controller
// ============================================================

type ResourceGroupController struct {
	service *devicesvc.ResourceGroupService
	camRepo devicesvc.CameraRepo
}

func NewResourceGroupController(
	service *devicesvc.ResourceGroupService,
	camRepo devicesvc.CameraRepo,
) *ResourceGroupController {
	return &ResourceGroupController{service: service, camRepo: camRepo}
}

func (ctrl *ResourceGroupController) mustLocals(c *fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ============================================================
// POST /resources/groups
// @Summary      Create resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      CreateResourceGroupRequest  true  "payload"
// @Success      201   {object}  gmod.ApiSuccessResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Failure      409   {object}  gmod.ApiErrorResponse
// @Router       /api/v1/resources/groups [post]
// ============================================================

type CreateResourceGroupRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ResourceType  string `json:"resourceType"`  // camera | sensor | "" = all
	MapVisibility string `json:"mapVisibility"` // public | private
}

func (ctrl *ResourceGroupController) Create(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "ResourceGroupController.Create")

	var body CreateResourceGroupRequest
	if err := c.BodyParser(&body); err != nil {
		log.Warn().Err(err).Msg("❌ invalid body")
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name is required"})
	}

	log.Info().
		Str("orgId", orgId).
		Str("name", body.Name).
		Str("resourceType", body.ResourceType).
		Str("mapVisibility", body.MapVisibility).
		Msg("📥 CreateResourceGroup")

	group, err := ctrl.service.CreateGroup(c.UserContext(), devicesvc.CreateGroupInput{
		TenantID:      tenantId,
		OrgID:         orgId,
		Name:          body.Name,
		Description:   body.Description,
		ResourceType:  body.ResourceType,
		MapVisibility: body.MapVisibility,
		CallerID:      callerUserId,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ CreateResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", group.GroupID).Msg("✅ ResourceGroup created")

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "resource group created successfully",
		"status":  true,
		"details": group,
	})
}

// ============================================================
// GET /resources/groups
// @Summary      List resource groups
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Produce      json
// @Param        search        query  string  false  "search by name"
// @Param        resourceType  query  string  false  "filter by resource type (camera|sensor)"
// @Param        page          query  int     false  "page"      default(1)
// @Param        perPages      query  int     false  "per page"  default(10)
// @Param        sortField     query  string  false  "sort field"
// @Param        sortOrder     query  string  false  "asc|desc"
// @Success      200  {object}  gmod.ApiSuccessResponse
// @Router       /api/v1/resources/groups [get]
// ============================================================

func (ctrl *ResourceGroupController) List(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)

	groups, total, err := ctrl.service.ListGroups(c.UserContext(), devicesvc.ListGroupsInput{
		TenantID:     tenantId,
		OrgID:        orgId,
		Search:       c.Query("search"),
		ResourceType: c.Query("resourceType"),
		Page:         c.QueryInt("page", 1),
		PerPages:     c.QueryInt("perPages", 10),
		SortField:    c.Query("sortField", "createdAt"),
		SortOrder:    c.Query("sortOrder", "desc"),
	})
	if err != nil {
		return handleErr(c, err)
	}

	perPages := c.QueryInt("perPages", 10)
	totalPages := (int(total) + perPages - 1) / perPages

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "resource groups fetched successfully",
		"status":  true,
		"details": groups,
		"pagination": fiber.Map{
			"page":         c.QueryInt("page", 1),
			"perPages":     perPages,
			"totalRecords": total,
			"totalPages":   totalPages,
		},
	})
}

// ============================================================
// PATCH /resources/groups/:id
// @Summary      Update resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string             true  "groupId"
// @Param        body  body   UpdateGroupRequest  true  "payload"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Failure      404   {object}  gmod.ApiErrorResponse
// @Router       /api/v1/resources/groups/{id} [patch]
// ============================================================

func (ctrl *ResourceGroupController) Update(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("id")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "ResourceGroupController.Update")

	var body UpdateGroupRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "invalid body", Status: false,
		})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "name is required", Status: false,
		})
	}
	if body.MapVisibility != "" && body.MapVisibility != "public" && body.MapVisibility != "private" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "mapVisibility must be public or private", Status: false,
		})
	}

	if err := ctrl.service.Update(c.UserContext(), devicesvc.UpdateGroupInput{
		TenantID:      tenantId,
		OrgID:         orgId,
		CallerID:      callerUserId,
		GroupID:       groupId,
		Name:          body.Name,
		Description:   body.Description,
		MapVisibility: body.MapVisibility,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ UpdateResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Msg("✅ ResourceGroup updated")
	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "resource group updated successfully",
		Status:  true,
	})
}

// ============================================================
// DELETE /resources/groups/:id
// @Summary      Delete resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Param        id   path  string  true  "groupId"
// @Success      200  {object}  gmod.ApiSuccessResponse
// @Failure      403  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/resources/groups/{id} [delete]
// ============================================================

func (ctrl *ResourceGroupController) Delete(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("id")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "ResourceGroupController.Delete")

	if err := ctrl.service.DeleteGroup(c.UserContext(), devicesvc.DeleteGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ DeleteResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Msg("✅ ResourceGroup deleted")
	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "resource group deleted successfully",
		"status":  true,
	})
}

// ============================================================
// POST /resources/groups/:groupId/devices
// ============================================================

type AddDeviceToGroupRequest struct {
	DeviceID string `json:"deviceId"`
}

func (ctrl *ResourceGroupController) AddDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AddDeviceToGroupRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.DeviceID == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "deviceId is required"})
	}

	if err := ctrl.service.AddDeviceToGroup(c.UserContext(), devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: body.DeviceID,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "device added to group successfully", "status": true,
	})
}

// ============================================================
// DELETE /resources/groups/:groupId/devices/:deviceId
// ============================================================

func (ctrl *ResourceGroupController) RemoveDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	deviceId := c.Params("deviceId")

	if err := ctrl.service.RemoveDeviceFromGroup(c.UserContext(), devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: deviceId,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "device removed from group successfully", "status": true,
	})
}

// ============================================================
// POST /resources/groups/:groupId/assignOu
// ============================================================

type AssignGroupOURequest struct {
	OUID     string `json:"ouId"`
	Relation string `json:"relation"` // viewer | editor | deleter
}

func (ctrl *ResourceGroupController) AssignOU(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AssignGroupOURequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.OUID == "" || body.Relation == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "ouId and relation are required"})
	}

	if err := ctrl.service.AssignGroupToOU(c.UserContext(), devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "group assigned to OU successfully", "status": true,
	})
}

// ============================================================
// DELETE /resources/groups/:groupId/assignOu
// ============================================================

func (ctrl *ResourceGroupController) RemoveOU(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AssignGroupOURequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}

	if err := ctrl.service.RemoveGroupFromOU(c.UserContext(), devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "group removed from OU successfully", "status": true,
	})
}

// ============================================================
// POST /devices
// ============================================================

func (ctrl *ResourceGroupController) CreateDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "ResourceGroupController.CreateDevice")

	var body devmod.Device
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}

	deviceId, err := ctrl.service.CreateDevice(c.UserContext(), devicesvc.CreateDeviceInput{
		TenantID: tenantId,
		OrgID:    orgId,
		CallerID: callerUserId,
		Device:   body,
	}, ctrl.camRepo)
	if err != nil {
		log.Error().Err(err).Msg("❌ CreateDevice failed")
		return handleErr(c, err)
	}

	log.Info().Str("deviceId", deviceId).Msg("✅ Device created")
	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device created successfully",
		"status":  true,
		"details": fiber.Map{"deviceId": deviceId, "permifySynced": true},
	})
}
