// controllers/deviceapi/deviceGroupMembers.go
package deviceapi

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// validResource — resourceType ที่รองรับใน URL param :resource
var supportedResources = map[string]bool{
	"camera": true,
}

func validResource(r string) bool {
	return supportedResources[r]
}

// ============================================================
// Request / Response types
// ============================================================

// CameraItem — item ใน body array สำหรับ resource type "camera"
type CameraItem struct {
	CamID string `json:"camId"`
}

type DeviceBulkResponse struct {
	Code    string                            `json:"code"`
	Message string                            `json:"message"`
	Status  bool                              `json:"status"`
	Details []devicesvc.GroupDeviceBulkResult `json:"details"`
}

// parseResourceItems — อ่าน body `{ "<resource>": [{"camId": "..."}] }`
// แล้ว convert เป็น []GroupDeviceItem
func parseResourceItems(c *fiber.Ctx, resource string) ([]devicesvc.GroupDeviceItem, error) {
	var body map[string][]CameraItem
	if err := c.BodyParser(&body); err != nil {
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
// @Router       /api/v1/devices/groups/{groupId}/devices [post]
// ============================================================

func (ctrl *ResourceGroupController) AddDevices(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	resource := c.Params("resource")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.AddDevices")

	if !validResource(resource) {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "unsupported resource type: " + resource})
	}

	devices, err := parseResourceItems(c, resource)
	if err != nil {
		log.Warn().Err(err).Msg("❌ invalid body")
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if len(devices) == 0 {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: resource + " list is required"})
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("groupId", groupId).
		Str("resource", resource).
		Int("count", len(devices)).
		Msg("📥 AddDevices request")

	results, inserted, duplicates, err := ctrl.service.AddDevicesToGroup(
		c.UserContext(),
		tenantId,
		orgId,
		groupId,
		callerUserId,
		devices,
		ctrl.camRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ AddDevicesToGroup failed")
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
		Msg("✅ AddDevices complete")

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
// @Router       /api/v1/devices/groups/{groupId}/devices [patch]
// ============================================================

func (ctrl *ResourceGroupController) RemoveDevices(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	groupId := c.Params("groupId")
	resource := c.Params("resource")
	log := logger.FromCtx(c.UserContext(), "deviceapi", "DeviceGroupController.RemoveDevices")

	if !validResource(resource) {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "unsupported resource type: " + resource})
	}

	devices, err := parseResourceItems(c, resource)
	if err != nil {
		log.Warn().Err(err).Msg("❌ invalid body")
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if len(devices) == 0 {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: resource + " list is required"})
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("groupId", groupId).
		Str("resource", resource).
		Int("count", len(devices)).
		Msg("📥 RemoveDevices request")

	results, removed, err := ctrl.service.RemoveDevicesFromGroup(
		c.UserContext(),
		tenantId,
		orgId,
		groupId,
		callerUserId,
		devices,
		ctrl.camRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("groupId", groupId).Msg("❌ RemoveDevicesFromGroup failed")
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
		Msg("✅ RemoveDevices complete")

	return c.JSON(DeviceBulkResponse{
		Code:    string(gmod.CodeSuccess),
		Message: buildDeviceBulkMessage(0, removed, 0, errorCount),
		Status:  true,
		Details: results,
	})
}