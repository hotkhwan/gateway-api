// internal/services/devicesvc/deviceGroupMembers.go
package devicesvc

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

// ============================================================
// Types
// ============================================================

type GroupDeviceItem struct {
	DeviceID string `json:"deviceId"`
}

type GroupDeviceBulkResult struct {
	DeviceID string `json:"deviceId"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ============================================================
// AddDevicesToGroup — bulk add
// ============================================================

func (s *ResourceGroupService) AddDevicesToGroup(
	ctx context.Context,
	tenantId string,
	orgId string,
	groupId string,
	callerUserId string,
	devices []GroupDeviceItem,
	camRepo CameraRepo,
) ([]GroupDeviceBulkResult, int, int, error) {

	log := logger.FromCtx(ctx, "devicesvc", "AddDevicesToGroup")

	if tenantId == "" || orgId == "" || groupId == "" || callerUserId == "" {
		return nil, 0, 0, ErrInvalidArgs
	}

	// 1) Permission check
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx,
		tenantId,
		config.CurrentSchemaVersion,
		"organization",
		orgId,
		"manage",
		"user",
		callerUserId,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return nil, 0, 0, ErrForbidden
	}

	// 2) Verify group belongs to org (cross-org guard)
	group, err := s.groupRepo.FindByIDAndOrg(ctx, groupId, tenantId, orgId)
	if err != nil {
		return nil, 0, 0, err
	}

	results := make([]GroupDeviceBulkResult, 0, len(devices))
	tuples := make([]map[string]any, 0, len(devices))
	inserted := 0
	duplicates := 0

	for _, d := range devices {
		// cross-org check
		deviceOrgID, err := camRepo.GetOrgID(ctx, d.DeviceID)
		if err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "device not found",
			})
			continue
		}
		if deviceOrgID != orgId {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "device does not belong to this organization",
			})
			continue
		}

		// resourceType filter: ถ้า group กำหนด resourceType ให้ตรวจ match
		if group.ResourceType != "" {
			devType, err := camRepo.GetDeviceType(ctx, d.DeviceID)
			if err == nil && devType != group.ResourceType {
				results = append(results, GroupDeviceBulkResult{
					DeviceID: d.DeviceID,
					Success:  false,
					Error:    fmt.Sprintf("device type '%s' does not match group resourceType '%s'", devType, group.ResourceType),
				})
				continue
			}
		}

		tuples = append(tuples, tupleDeviceParentGroup(d.DeviceID, groupId))
		results = append(results, GroupDeviceBulkResult{DeviceID: d.DeviceID, Success: true})
		inserted++
	}

	// Permify batch write
	if len(tuples) > 0 {
		if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
			return nil, 0, 0, fmt.Errorf("%w: %v", ErrPermifySyncFailed, err)
		}
	}

	log.Info().
		Str("groupId", groupId).
		Int("inserted", inserted).
		Int("duplicates", duplicates).
		Msg("✅ AddDevicesToGroup complete")

	return results, inserted, duplicates, nil
}

// ============================================================
// RemoveDevicesFromGroup — bulk remove
// ============================================================

func (s *ResourceGroupService) RemoveDevicesFromGroup(
	ctx context.Context,
	tenantId string,
	orgId string,
	groupId string,
	callerUserId string,
	devices []GroupDeviceItem,
	camRepo CameraRepo,
) ([]GroupDeviceBulkResult, int, error) {

	log := logger.FromCtx(ctx, "devicesvc", "RemoveDevicesFromGroup")

	if tenantId == "" || orgId == "" || groupId == "" || callerUserId == "" {
		return nil, 0, ErrInvalidArgs
	}

	// 1) Permission check
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx,
		tenantId,
		config.CurrentSchemaVersion,
		"organization",
		orgId,
		"manage",
		"user",
		callerUserId,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return nil, 0, ErrForbidden
	}

	// 2) Verify group belongs to org
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, groupId, tenantId, orgId); err != nil {
		return nil, 0, err
	}

	results := make([]GroupDeviceBulkResult, 0, len(devices))
	removed := 0

	for _, d := range devices {
		if err := s.authzClient.DeleteSpecificTupleWithRelation(
			ctx, tenantId,
			"device", d.DeviceID, "parentGroup",
			"resourceGroup", groupId,
		); err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    err.Error(),
			})
			continue
		}

		results = append(results, GroupDeviceBulkResult{DeviceID: d.DeviceID, Success: true})
		removed++
	}

	log.Info().
		Str("groupId", groupId).
		Int("removed", removed).
		Msg("✅ RemoveDevicesFromGroup complete")

	return results, removed, nil
}
