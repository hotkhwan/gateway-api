// internal/services/devicesvc/deviceGroupOps.go
package devicesvc

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/models/devmod"
)

var validOURelations = map[string]bool{
	"viewer":  true,
	"editor":  true,
	"deleter": true,
}

func (s *ResourceGroupService) AddDeviceToGroup(ctx context.Context, input AddDeviceToGroupInput, camRepo CameraRepo) error {
	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return err
	}
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, input.GroupID, input.TenantID, input.OrgID); err != nil {
		return fmt.Errorf("group validation: %w", err)
	}
	deviceOrgID, err := camRepo.GetOrgID(ctx, input.DeviceID)
	if err != nil {
		return fmt.Errorf("device not found: %w", err)
	}
	if deviceOrgID != input.OrgID {
		return fmt.Errorf("cross-org injection blocked: device orgId mismatch")
	}
	if err := s.authzClient.WriteTuples(ctx, input.TenantID, []map[string]any{
		tupleDeviceParentGroup(input.DeviceID, input.GroupID),
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrPermifySyncFailed, err)
	}
	return nil
}

func (s *ResourceGroupService) RemoveDeviceFromGroup(ctx context.Context, input AddDeviceToGroupInput, camRepo CameraRepo) error {
	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return err
	}
	return s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, input.TenantID,
		"device", input.DeviceID, "parentGroup",
		"resourceGroup", input.GroupID,
	)
}

func (s *ResourceGroupService) AssignGroupToOU(ctx context.Context, input AssignGroupToOUInput) error {
	if !validOURelations[input.Relation] {
		return fmt.Errorf("invalid relation '%s': must be viewer|editor|deleter", input.Relation)
	}
	if ouErr := s.guardManageOU(ctx, input.TenantID, input.OUID, input.CallerID); ouErr != nil {
		if orgErr := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); orgErr != nil {
			return ErrForbidden
		}
	}
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, input.GroupID, input.TenantID, input.OrgID); err != nil {
		return fmt.Errorf("group validation: %w", err)
	}
	return s.authzClient.WriteTuples(ctx, input.TenantID, []map[string]any{
		tupleGroupOU(input.GroupID, input.OUID, input.Relation),
	})
}

func (s *ResourceGroupService) RemoveGroupFromOU(ctx context.Context, input AssignGroupToOUInput) error {
	if ouErr := s.guardManageOU(ctx, input.TenantID, input.OUID, input.CallerID); ouErr != nil {
		if orgErr := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); orgErr != nil {
			return ErrForbidden
		}
	}
	return s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, input.TenantID,
		"resourceGroup", input.GroupID, input.Relation,
		"orgUnit", input.OUID,
	)
}

// CreateDevice — ✅ signature ใหม่ + ✅ Roi convert [][]map[string]string → []interface{}
func (s *ResourceGroupService) CreateDevice(ctx context.Context, input CreateDeviceInput, camRepo CameraRepo) (string, error) {
	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return "", err
	}

	// devmod.Device.Roi = [][]map[string]string → ต้อง convert เป็น []interface{}
	roi := make([]interface{}, 0, len(input.Device.Roi))
	for _, row := range input.Device.Roi {
		roi = append(roi, row)
	}

	deviceId, err := camRepo.Insert(ctx, devmod.CreateCameraInput{
		TenantID: input.TenantID,
		OrgID:    input.OrgID,
		CallerID: input.CallerID,
		Name:     input.Device.Name,
		User:     input.Device.User,
		Password: input.Device.Password,
		URL:      input.Device.URL,
		District: input.Device.District,
		Lat:      input.Device.Lat,
		Lng:      input.Device.Lng,
		Roi:      roi,
	})
	if err != nil {
		return "", err
	}

	if err := s.authzClient.WriteTuples(ctx, input.TenantID, []map[string]any{
		tupleDeviceParentOrg(deviceId, input.OrgID),
	}); err != nil {
		_ = camRepo.Delete(ctx, deviceId)
		return "", fmt.Errorf("%w: %v", ErrPermifySyncFailed, err)
	}
	return deviceId, nil
}
