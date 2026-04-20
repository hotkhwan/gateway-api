// controllers/deviceapi/deviceGroupMembers.go
package deviceapi

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// validResource — resourceType that is supported in URL param :resource
var supportedResources = map[string]bool{
	"camera": true,
}

func validResource(r string) bool {
	return supportedResources[r]
}

// ============================================================
// Request / Response types
// ============================================================

// CameraItem — item in body array for resource type "camera"
type CameraItem struct {
	CamID string `json:"camId"`
}

type DeviceBulkResponse struct {
	Code    string                            `json:"code"`
	Message string                            `json:"message"`
	Status  bool                              `json:"status"`
	Details []devicesvc.GroupDeviceBulkResult `json:"details"`
}

// parseResourceItems — reads body `{ "<resource>": [{"camId": "..."}] }`
// and converts to []GroupDeviceItem
func parseResourceItems(c fiber.Ctx, resource string) ([]devicesvc.GroupDeviceItem, error) {
	var body map[string][]CameraItem
	if err := c.Bind().Body(&body); err != nil {
		return nil, err
	}
	items, ok := body[resource]
	if !ok || len(items) == 0 {
		return nil, nil
	}
	result := make([]devicesvc.GroupDeviceItem, 0, len(items))
	for _, item := range items {
		if item.CamID != "" {
			result = append(result, devicesvc.GroupDeviceItem{DeviceID: item.CamID})
		}
	}
	return result, nil
}

func buildDeviceBulkMessage(inserted, removed, duplicates, errors int) string {
	return fmt.Sprintf("insert %d, remove %d, duplicate %d, error %d", inserted, removed, duplicates, errors)
}

// ============================================================
// GET /resources/groups/:groupId/:resource
// @Summary      List cameras in resource group
// @Tags         ResourceGroups
// @Security     BearerAuth
// @Produce      json
// @Param        groupId   path   string  true   "group id"
// @Param        resource  path   string  true   "resource type (camera)"
// @Param        search    query  string  false  "search by name"
// @Param        page      query  int     false  "page"      default(1)
// @Param        perPage   query  int     false  "per page"  default(10)
// @Param        sortField query  string  false  "sort field"
// @Param        sortOrder query  string  false  "asc|desc"
// @Success      200  {object}  map[string]interface{}
// @Router       /resources/groups/{groupId}/{resource} [get]
// ============================================================

func (ctrl *ResourceGroupController) ListCameras(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.ListCameras", "deviceapi", "ListCameras")
	defer end()

	tenantId, workspaceId, _ := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	resource := c.Params("resource")

	if !validResource(resource) {
		return httputil.FailBadRequest(c, "unsupported resource type: "+resource)
	}

	result, err := ctrl.service.ListCamerasInGroup(
		c,
		tenantId, workspaceId, groupId,
		devicesvc.ListInGroupParams{
			Search:    c.Query("search"),
			Page:      fiber.Query[int](c, "page", 1),
			PerPage:   fiber.Query[int](c, "perPage", 10),
			SortField: c.Query("sortField", "createAt"),
			SortOrder: c.Query("sortOrder", "desc"),
		},
		ctrl.camRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("ListCamerasInGroup failed")
		return handleErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "cameras fetched successfully",
		"status":  true,
		"details": result.Items,
		"pagination": fiber.Map{
			"page":         result.Page,
			"perPage":      result.PerPage,
			"totalRecords": result.TotalRecords,
			"totalPages":   result.TotalPages,
		},
	})
}

// ============================================================
// POST /devices/groups/:groupId/devices
// @Summary      Add devices to group (bulk)
// @Description  Bulk add devices into a device group. Validates cross-org and deviceType match.
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        groupId  path      string            true  "group id"
// @Param        body     body      AddDevicesRequest true  "devices payload"
// @Success      200      {object}  DeviceBulkResponse
// @Failure      400      {object}  gmod.ApiErrorResponse
// @Failure      403      {object}  gmod.ApiErrorResponse
// @Router       /devices/groups/{groupId}/devices [post]
// ============================================================

func (ctrl *ResourceGroupController) AddDevices(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.AddDevices", "deviceapi", "AddDevices")
	defer end()

	tenantId, workspaceId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	resource := c.Params("resource")

	if !validResource(resource) {
		return httputil.FailBadRequest(c, "unsupported resource type: "+resource)
	}

	devices, err := parseResourceItems(c, resource)
	if err != nil {
		log.Warn().Err(err).Msg("invalid body")
		return httputil.FailBadRequest(c, "invalid body")
	}
	if len(devices) == 0 {
		return httputil.FailBadRequest(c, resource+" list is required")
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("workspaceId", workspaceId).
		Str("groupId", groupId).
		Str("resource", resource).
		Int("count", len(devices)).
		Msg("AddDevices request")

	results, inserted, duplicates, err := ctrl.service.AddDevicesToGroup(
		c,
		tenantId,
		workspaceId,
		groupId,
		callerUserId,
		devices,
		ctrl.camRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("AddDevicesToGroup failed")
		return handleErr(c, err)
	}

	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "device already in group" && r.Error != "" {
			errorCount++
		}
	}

	log.Info().
		Str("groupId", groupId).
		Int("inserted", inserted).
		Int("duplicates", duplicates).
		Int("errors", errorCount).
		Msg("AddDevices complete")

	return c.JSON(DeviceBulkResponse{
		Code:    string(gmod.CodeSuccess),
		Message: buildDeviceBulkMessage(inserted, 0, duplicates, errorCount),
		Status:  true,
		Details: results,
	})
}

// ============================================================
// PATCH /devices/groups/:groupId/devices
// @Summary      Remove devices from group (bulk)
// @Description  Bulk remove devices from a device group.
// @Tags         DeviceGroups
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        groupId  path      string               true  "group id"
// @Param        body     body      RemoveDevicesRequest true  "devices payload"
// @Success      200      {object}  DeviceBulkResponse
// @Failure      400      {object}  gmod.ApiErrorResponse
// @Failure      403      {object}  gmod.ApiErrorResponse
// @Router       /devices/groups/{groupId}/devices [patch]
// ============================================================

func (ctrl *ResourceGroupController) RemoveDevices(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "ResourceGroupController.RemoveDevices", "deviceapi", "RemoveDevices")
	defer end()

	tenantId, workspaceId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	resource := c.Params("resource")

	if !validResource(resource) {
		return httputil.FailBadRequest(c, "unsupported resource type: "+resource)
	}

	devices, err := parseResourceItems(c, resource)
	if err != nil {
		log.Warn().Err(err).Msg("invalid body")
		return httputil.FailBadRequest(c, "invalid body")
	}
	if len(devices) == 0 {
		return httputil.FailBadRequest(c, resource+" list is required")
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("workspaceId", workspaceId).
		Str("groupId", groupId).
		Str("resource", resource).
		Int("count", len(devices)).
		Msg("RemoveDevices request")

	results, removed, err := ctrl.service.RemoveDevicesFromGroup(
		c,
		tenantId,
		workspaceId,
		groupId,
		callerUserId,
		devices,
		ctrl.camRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("RemoveDevicesFromGroup failed")
		return handleErr(c, err)
	}

	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "device not in group" && r.Error != "" {
			errorCount++
		}
	}

	log.Info().
		Str("groupId", groupId).
		Int("removed", removed).
		Int("errors", errorCount).
		Msg("RemoveDevices complete")

	return c.JSON(DeviceBulkResponse{
		Code:    string(gmod.CodeSuccess),
		Message: buildDeviceBulkMessage(0, removed, 0, errorCount),
		Status:  true,
		Details: results,
	})
}
