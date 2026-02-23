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
// AddDevicesToGroup — bulk add (ล้อ AssignUsersToOU)
// ============================================================

func (s *DeviceGroupService) AddDevicesToGroup(
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

	// 3) Get existing devices in group from Permify
	rels, err := s.authzClient.ListEntityRelationships(ctx, tenantId, "deviceGroup", groupId)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list relationships error: %w", err)
	}

	// build existing set: deviceId → true
	existing := make(map[string]bool)
	for _, r := range rels {
		if r.Relation == "parentGroup" {
			// subject is device:<id> but stored as entity side in our tuple
			// ตรวจจาก device#parentGroup@deviceGroup:<groupId>
			// so we read subject type=device
		}
		// ✅ tuple: device#parentGroup@deviceGroup → subject.type=deviceGroup, entity.type=device
		// ListEntityRelationships ดึงโดย entity=deviceGroup ดังนั้นได้ tuples ที่ deviceGroup เป็น entity
		// ที่ถูกต้องคือ: device(entity)#parentGroup@deviceGroup(subject) → ต้องดึงจาก entity=device
		// ดังนั้นใช้ camRepo เช็ค groupId แทน
		_ = r
	}
	// ✅ ใช้ camRepo ตรวจซ้ำจาก mongo แทน (groupIds field)
	_ = existing

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

		// deviceType filter: ถ้า group กำหนด deviceType ให้ตรวจ match
		if group.DeviceType != "" {
			devType, err := camRepo.GetDeviceType(ctx, d.DeviceID)
			if err == nil && devType != group.DeviceType {
				results = append(results, GroupDeviceBulkResult{
					DeviceID: d.DeviceID,
					Success:  false,
					Error:    fmt.Sprintf("device type '%s' does not match group type '%s'", devType, group.DeviceType),
				})
				continue
			}
		}

		// duplicate check (mongo)
		isDup, err := camRepo.HasGroupID(ctx, d.DeviceID, groupId)
		if err == nil && isDup {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "device already in group",
			})
			duplicates++
			continue
		}

		// Mongo $addToSet
		if err := camRepo.AddGroupID(ctx, d.DeviceID, groupId); err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    err.Error(),
			})
			continue
		}

		tuples = append(tuples, tupleDeviceParentGroup(d.DeviceID, groupId))
		results = append(results, GroupDeviceBulkResult{DeviceID: d.DeviceID, Success: true})
		inserted++
	}

	// Permify batch write
	if len(tuples) > 0 {
		if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
			// rollback mongo on permify fail
			for _, t := range tuples {
				entity := t["entity"].(map[string]any)
				_ = camRepo.RemoveGroupID(ctx, entity["id"].(string), groupId)
			}
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
// RemoveDevicesFromGroup — bulk remove (ล้อ RemoveUsersFromOU)
// ============================================================

func (s *DeviceGroupService) RemoveDevicesFromGroup(
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
		// ตรวจว่า device อยู่ใน group จริง
		isDup, err := camRepo.HasGroupID(ctx, d.DeviceID, groupId)
		if err != nil || !isDup {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "device not in group",
			})
			continue
		}

		// Permify delete ก่อน
		if err := s.authzClient.DeleteSpecificTupleWithRelation(
			ctx, tenantId,
			"device", d.DeviceID, "parentGroup",
			"deviceGroup", groupId,
		); err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    err.Error(),
			})
			continue
		}

		// Mongo $pull
		if err := camRepo.RemoveGroupID(ctx, d.DeviceID, groupId); err != nil {
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
