// controllers/deviceapi/deviceGroup.go
package deviceapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
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

func (ctrl *ResourceGroupController) mustLocals(c fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeWorkspace").(string)
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
// @Success      201   {object}  gmod.SuccessDetailsResponse
// @Failure      400   {object}  gmod.ApiErrorResponse
// @Failure      403   {object}  gmod.ApiErrorResponse
// @Failure      409   {object}  gmod.ApiErrorResponse
// @Router       /resources/groups [post]
// ============================================================

type CreateResourceGroupRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ResourceType  string `json:"resourceType"`  // camera | sensor | "" = all
	MapVisibility string `json:"mapVisibility"` // public | private
}

func (ctrl *ResourceGroupController) Create(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.Create", "deviceapi", "Create")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)

	var body CreateResourceGroupRequest
	if err := c.Bind().Body(&body); err != nil {
		log.Warn().Err(err).Msg("invalid body")
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name is required")
	}

	log.Info().
		Str("orgId", orgId).
		Str("name", body.Name).
		Str("resourceType", body.ResourceType).
		Str("mapVisibility", body.MapVisibility).
		Msg("CreateResourceGroup")

	group, err := ctrl.service.CreateGroup(c, devicesvc.CreateGroupInput{
		TenantID:      tenantId,
		OrgID:         orgId,
		Name:          body.Name,
		Description:   body.Description,
		ResourceType:  body.ResourceType,
		MapVisibility: body.MapVisibility,
		CallerID:      callerUserId,
	})
	if err != nil {
		log.Error().Err(err).Msg("CreateResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", group.GroupID).Msg("ResourceGroup created")
	return httputil.Created(c, group, "resource group created successfully")
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
// @Success      200  {object}  gmod.PaginatedResponse
// @Router       /resources/groups [get]
// ============================================================

func (ctrl *ResourceGroupController) List(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.List", "deviceapi", "List")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)

	groups, total, err := ctrl.service.ListGroups(c, devicesvc.ListGroupsInput{
		TenantID:     tenantId,
		OrgID:        orgId,
		Search:       c.Query("search"),
		ResourceType: c.Query("resourceType"),
		Page:         fiber.Query[int](c, "page", 1),
		PerPages:     fiber.Query[int](c, "perPages", 10),
		SortField:    c.Query("sortField", "createdAt"),
		SortOrder:    c.Query("sortOrder", "desc"),
	})
	if err != nil {
		log.Error().Err(err).Msg("ListGroups failed")
		return handleErr(c, err)
	}

	perPages := fiber.Query[int](c, "perPages", 10)
	totalPages := (int(total) + perPages - 1) / perPages

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "resource groups fetched successfully",
		"status":  true,
		"details": groups,
		"pagination": fiber.Map{
			"page":         fiber.Query[int](c, "page", 1),
			"perPages":     perPages,
			"totalRecords": int(total),
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
// @Router       /resources/groups/{id} [patch]
// ============================================================

func (ctrl *ResourceGroupController) Update(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.Update", "deviceapi", "Update")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("id")

	var body UpdateGroupRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Name == "" {
		return httputil.FailBadRequest(c, "name is required")
	}
	if body.MapVisibility != "" && body.MapVisibility != "public" && body.MapVisibility != "private" {
		return httputil.FailBadRequest(c, "mapVisibility must be public or private")
	}

	if err := ctrl.service.Update(c, devicesvc.UpdateGroupInput{
		TenantID:      tenantId,
		OrgID:         orgId,
		CallerID:      callerUserId,
		GroupID:       groupId,
		Name:          body.Name,
		Description:   body.Description,
		MapVisibility: body.MapVisibility,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("UpdateResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Msg("ResourceGroup updated")
	return httputil.MessageOK(c, "resource group updated successfully")
}

// ============================================================
// DELETE /resources/groups/:id
// @Summary      Delete resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Param        id   path  string  true  "groupId"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      403  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /resources/groups/{id} [delete]
// ============================================================

func (ctrl *ResourceGroupController) Delete(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.Delete", "deviceapi", "Delete")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("id")

	if err := ctrl.service.DeleteGroup(c, devicesvc.DeleteGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("DeleteResourceGroup failed")
		return handleErr(c, err)
	}

	log.Info().Str("groupId", groupId).Msg("ResourceGroup deleted")
	return httputil.MessageOK(c, "resource group deleted successfully")
}

// ============================================================
// POST /resources/groups/:groupId/devices
// ============================================================

type AddDeviceToGroupRequest struct {
	DeviceID string `json:"deviceId"`
}

func (ctrl *ResourceGroupController) AddDevice(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.AddDevice", "deviceapi", "AddDevice")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AddDeviceToGroupRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.DeviceID == "" {
		return httputil.FailBadRequest(c, "deviceId is required")
	}

	if err := ctrl.service.AddDeviceToGroup(c, devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: body.DeviceID,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("AddDevice failed")
		return handleErr(c, err)
	}

	return httputil.MessageOK(c, "device added to group successfully")
}

// ============================================================
// DELETE /resources/groups/:groupId/devices/:deviceId
// ============================================================

func (ctrl *ResourceGroupController) RemoveDevice(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.RemoveDevice", "deviceapi", "RemoveDevice")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	deviceId := c.Params("deviceId")

	if err := ctrl.service.RemoveDeviceFromGroup(c, devicesvc.AddDeviceToGroupInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		DeviceID: deviceId,
		CallerID: callerUserId,
	}, ctrl.camRepo); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("RemoveDevice failed")
		return handleErr(c, err)
	}

	return httputil.MessageOK(c, "device removed from group successfully")
}

// ============================================================
// POST /resources/groups/:groupId/assignOu
// ============================================================

type AssignGroupOURequest struct {
	OUID     string `json:"ouId"`
	Relation string `json:"relation"` // viewer | editor | deleter
}

func (ctrl *ResourceGroupController) AssignOU(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.AssignOU", "deviceapi", "AssignOU")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AssignGroupOURequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.OUID == "" || body.Relation == "" {
		return httputil.FailBadRequest(c, "ouId and relation are required")
	}

	if err := ctrl.service.AssignGroupToOU(c, devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("AssignOU failed")
		return handleErr(c, err)
	}

	return httputil.MessageOK(c, "group assigned to OU successfully")
}

// ============================================================
// DELETE /resources/groups/:groupId/assignOu
// ============================================================

func (ctrl *ResourceGroupController) RemoveOU(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.RemoveOU", "deviceapi", "RemoveOU")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")

	var body AssignGroupOURequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	if err := ctrl.service.RemoveGroupFromOU(c, devicesvc.AssignGroupToOUInput{
		TenantID: tenantId,
		OrgID:    orgId,
		GroupID:  groupId,
		OUID:     body.OUID,
		Relation: body.Relation,
		CallerID: callerUserId,
	}); err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("RemoveOU failed")
		return handleErr(c, err)
	}

	return httputil.MessageOK(c, "group removed from OU successfully")
}

// ============================================================
// POST /devices
// ============================================================

func (ctrl *ResourceGroupController) CreateDevice(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.CreateDevice", "deviceapi", "CreateDevice")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)

	var body devmod.Device
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	deviceId, err := ctrl.service.CreateDevice(c, devicesvc.CreateDeviceInput{
		TenantID: tenantId,
		OrgID:    orgId,
		CallerID: callerUserId,
		Device:   body,
	}, ctrl.camRepo)
	if err != nil {
		log.Error().Err(err).Msg("CreateDevice failed")
		return handleErr(c, err)
	}

	log.Info().Str("deviceId", deviceId).Msg("Device created")
	return httputil.Created(c, fiber.Map{"deviceId": deviceId, "permifySynced": true}, "device created successfully")
}
