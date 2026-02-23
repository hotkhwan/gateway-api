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

type DeviceGroupController struct {
	service *devicesvc.DeviceGroupService
	camRepo devicesvc.CameraRepo
}

func NewDeviceGroupController(
	service *devicesvc.DeviceGroupService,
	camRepo devicesvc.CameraRepo,
) *DeviceGroupController {
	return &DeviceGroupController{service: service, camRepo: camRepo}
}

func (ctrl *DeviceGroupController) mustLocals(c *fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// ============================================================
// POST /device-groups
// @Summary      Create device group
// @Description  Create a new device group under the active organization
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      CreateDeviceGroupRequest  true  "payload"
// @Success      201   {object}  gmod.ApiSuccessResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      401   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Failure      409   {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups [post]
// ============================================================

type CreateDeviceGroupRequest struct {
	Name        string `json:"name"        example:"Camera Zone A"`
	Description string `json:"description" example:"Main entrance cameras"`
	DeviceType  string `json:"deviceType"  example:"camera"` // camera | sensor | "" = all
	Public      bool   `json:"public"      example:"false"`
}

func (ctrl *DeviceGroupController) Create(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.Create")

	var body CreateDeviceGroupRequest
	if err := c.BodyParser(&body); err != nil {
		log.Warn().Err(err).Msg("❌ invalid body")
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name is required"})
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("callerUserId", callerUserId).
		Str("name", body.Name).
		Str("deviceType", body.DeviceType).
		Bool("public", body.Public).
		Msg("📥 CreateDeviceGroup request")

	group, err := ctrl.service.CreateGroup(c.UserContext(), devicesvc.CreateGroupInput{
		TenantID:    tenantId,
		OrgID:       orgId,
		Name:        body.Name,
		Description: body.Description,
		DeviceType:  body.DeviceType,
		Public:      body.Public,
		CallerID:    callerUserId,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ CreateDeviceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", group.ID.Hex()).Msg("✅ DeviceGroup created")

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device group created successfully",
		"status":  true,
		"details": group,
	})
}

// ============================================================
// GET /device-groups
// @Summary      List device groups
// @Description  List device groups in the active organization with search, filter and pagination
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Produce      json
// @Param        search      query     string  false  "search by name"
// @Param        deviceType  query     string  false  "filter by device type (camera|sensor)"
// @Param        page        query     int     false  "page number"       default(1)
// @Param        perPages    query     int     false  "items per page"    default(10)
// @Param        sortField   query     string  false  "sort field"        default(createdAt)
// @Param        sortOrder   query     string  false  "asc|desc"          default(desc)
// @Success      200  {object}  gmod.ApiSuccessResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups [get]
// ============================================================

func (ctrl *DeviceGroupController) List(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.List")

	search := c.Query("search")
	deviceType := c.Query("deviceType")
	page := c.QueryInt("page", 1)
	perPages := c.QueryInt("perPages", 10)
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	log.Debug().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("search", search).
		Str("deviceType", deviceType).
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Msg("📥 ListDeviceGroups request")

	groups, total, err := ctrl.service.ListGroups(c.UserContext(), devicesvc.ListGroupsInput{
		TenantID:   tenantId,
		OrgID:      orgId,
		Search:     search,
		DeviceType: deviceType,
		Page:       page,
		PerPages:   perPages,
		SortField:  sortField,
		SortOrder:  sortOrder,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ ListDeviceGroups failed")
		return handleErr(c, err)
	}

	totalPages := (int(total) + perPages - 1) / perPages

	log.Debug().
		Int64("total", total).
		Int("count", len(groups)).
		Msg("✅ ListDeviceGroups success")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device groups fetched successfully",
		"status":  true,
		"details": groups,
		"pagination": fiber.Map{
			"page":         page,
			"perPages":     perPages,
			"totalRecords": total,
			"totalPages":   totalPages,
			"sortField":    sortField,
			"sortOrder":    sortOrder,
		},
	})
}

// Update godoc
// @Summary      Update resource group
// @Description  Update name / description / mapVisibility of a resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path   string             true  "groupId (UUID)"
// @Param        body  body   UpdateGroupRequest  true  "payload"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Failure      404   {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{id} [patch]
func (ctrl *DeviceGroupController) Update(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	callerUserId, _ := c.Locals("userId").(string)
	groupId := c.Params("id")

	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.Update")

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

	log.Info().
		Str("groupId", groupId).
		Str("callerUserId", callerUserId).
		Str("name", body.Name).
		Str("mapVisibility", body.MapVisibility).
		Msg("📥 UpdateGroup")

	if err := ctrl.service.Update(c.UserContext(), devicesvc.UpdateGroupInput{
		TenantID:      tenantId,
		OrgID:         orgId,
		CallerID:      callerUserId,
		GroupID:       groupId,
		Name:          body.Name,
		Description:   body.Description,
		MapVisibility: body.MapVisibility,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ UpdateGroup failed")
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
// DELETE /device-groups/:id
// @Summary      Delete device group
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "group id"
// @Success      200  {object}  gmod.ApiSuccessResponse
// @Failure      403  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{id} [delete]
// ============================================================

func (ctrl *DeviceGroupController) Delete(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("id")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.Delete")

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("groupId", groupId).
		Str("callerUserId", callerUserId).
		Msg("📥 DeleteDeviceGroup request")

	if err := ctrl.service.DeleteGroup(c.UserContext(), devicesvc.DeleteGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ DeleteDeviceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Msg("✅ DeviceGroup deleted")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device group deleted successfully",
		"status":  true,
	})
}

// ============================================================
// POST /device-groups/:groupId/devices
// @Summary      Add device to group
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        groupId  path      string                 true  "group id"
// @Param        body     body      AddDeviceToGroupRequest true  "payload"
// @Success      200      {object}  gmod.ApiSuccessResponse
// @Failure      403      {object}  gmod.ApiErrorResponse
// @Failure      404      {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{groupId}/devices [post]
// ============================================================

type AddDeviceToGroupRequest struct {
	DeviceID string `json:"deviceId" example:"64a1f2b3c4d5e6f7a8b9c0d1"`
}

func (ctrl *DeviceGroupController) AddDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.AddDevice")

	var body AddDeviceToGroupRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.DeviceID == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "deviceId is required"})
	}

	log.Info().
		Str("groupId", groupId).
		Str("deviceId", body.DeviceID).
		Msg("📥 AddDeviceToGroup request")

	if err := ctrl.service.AddDeviceToGroup(c.UserContext(), devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: body.DeviceID,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		log.Error().Err(err).Msg("❌ AddDeviceToGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Str("deviceId", body.DeviceID).Msg("✅ Device added to group")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device added to group successfully",
		"status":  true,
	})
}

// ============================================================
// DELETE /device-groups/:groupId/devices/:deviceId
// @Summary      Remove device from group
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Produce      json
// @Param        groupId   path  string  true  "group id"
// @Param        deviceId  path  string  true  "device id"
// @Success      200  {object}  gmod.ApiSuccessResponse
// @Failure      403  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{groupId}/devices/{deviceId} [delete]
// ============================================================

func (ctrl *DeviceGroupController) RemoveDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	deviceId := c.Params("deviceId")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.RemoveDevice")

	log.Info().
		Str("groupId", groupId).
		Str("deviceId", deviceId).
		Msg("📥 RemoveDeviceFromGroup request")

	if err := ctrl.service.RemoveDeviceFromGroup(c.UserContext(), devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: deviceId,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		log.Error().Err(err).Msg("❌ RemoveDeviceFromGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Str("deviceId", deviceId).Msg("✅ Device removed from group")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device removed from group successfully",
		"status":  true,
	})
}

// ============================================================
// POST /device-groups/:groupId/assign-ou
// @Summary      Assign group to OU
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        groupId  path      string               true  "group id"
// @Param        body     body      AssignGroupOURequest true  "payload"
// @Success      200      {object}  gmod.ApiSuccessResponse
// @Failure      403      {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{groupId}/assign-ou [post]
// ============================================================

type AssignGroupOURequest struct {
	OUID     string `json:"ouId"     example:"ou-abc123"`
	Relation string `json:"relation" example:"viewer"` // viewer | editor | deleter
}

func (ctrl *DeviceGroupController) AssignOU(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.AssignOU")

	var body AssignGroupOURequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.OUID == "" || body.Relation == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "ouId and relation are required"})
	}

	log.Info().
		Str("groupId", groupId).
		Str("ouId", body.OUID).
		Str("relation", body.Relation).
		Msg("📥 AssignGroupToOU request")

	if err := ctrl.service.AssignGroupToOU(c.UserContext(), devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Msg("❌ AssignGroupToOU failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Str("ouId", body.OUID).Str("relation", body.Relation).Msg("✅ Group assigned to OU")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "group assigned to OU successfully",
		"status":  true,
	})
}

// ============================================================
// DELETE /device-groups/:groupId/assign-ou
// @Summary      Remove group from OU
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        groupId  path      string               true  "group id"
// @Param        body     body      AssignGroupOURequest true  "payload"
// @Success      200      {object}  gmod.ApiSuccessResponse
// @Failure      403      {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices/groups/{groupId}/assign-ou [delete]
// ============================================================

func (ctrl *DeviceGroupController) RemoveOU(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.RemoveOU")

	var body AssignGroupOURequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}

	log.Info().
		Str("groupId", groupId).
		Str("ouId", body.OUID).
		Str("relation", body.Relation).
		Msg("📥 RemoveGroupFromOU request")

	if err := ctrl.service.RemoveGroupFromOU(c.UserContext(), devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Msg("❌ RemoveGroupFromOU failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Str("ouId", body.OUID).Msg("✅ Group removed from OU")

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "group removed from OU successfully",
		"status":  true,
	})
}

// ============================================================
// POST /devices
// @Summary      Create device
// @Tags         Devices
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      devmod.Device  true  "device payload"
// @Success      201   {object}  gmod.ApiSuccessResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Router       /api/v1/devices [post]
// ============================================================

func (ctrl *DeviceGroupController) CreateDevice(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.CreateDevice")

	var body devmod.Device
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("callerUserId", callerUserId).
		Str("deviceName", body.Name).
		Msg("📥 CreateDevice request")

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

	log.Info().Str("deviceId", deviceId).Str("orgId", orgId).Msg("✅ Device created and Permify synced")

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "device created successfully",
		"status":  true,
		"details": fiber.Map{
			"deviceId":      deviceId,
			"permifySynced": true,
		},
	})
}
