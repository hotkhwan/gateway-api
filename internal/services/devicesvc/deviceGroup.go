// internal/services/devicesvc/deviceGroup.go
package devicesvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ============================================================
// Service struct + constructor
// ============================================================

type DeviceGroupService struct {
	groupRepo   *devicerepo.DeviceGroupRepo
	authzClient authzgw.Client
}

func NewDeviceGroupService(
	groupRepo *devicerepo.DeviceGroupRepo,
	authzClient authzgw.Client,
) *DeviceGroupService {
	return &DeviceGroupService{
		groupRepo:   groupRepo,
		authzClient: authzClient,
	}
}

// ============================================================
// Permission guards
// ============================================================

func (s *DeviceGroupService) guardManageOrg(ctx context.Context, tenantId, orgId, callerUserId string) error {
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, "", "organization", orgId, "manage", "user", callerUserId,
	)
	if err != nil {
		return fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *DeviceGroupService) guardManageOU(ctx context.Context, tenantId, ouId, callerUserId string) error {
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, "", "orgUnit", ouId, "manage", "user", callerUserId,
	)
	if err != nil {
		return fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// ============================================================
// Tuple builders
// ============================================================

func tupleDeviceGroupParentOrg(groupId, orgId string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "deviceGroup", "id": groupId},
		"relation": "parentOrg",
		"subject":  map[string]any{"type": "organization", "id": orgId},
	}
}

func tupleDeviceParentOrg(deviceId, orgId string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "device", "id": deviceId},
		"relation": "parentOrg",
		"subject":  map[string]any{"type": "organization", "id": orgId},
	}
}

func tupleDeviceParentGroup(deviceId, groupId string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "device", "id": deviceId},
		"relation": "parentGroup",
		"subject":  map[string]any{"type": "deviceGroup", "id": groupId},
	}
}

func tupleGroupOU(groupId, ouId, relation string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "deviceGroup", "id": groupId},
		"relation": relation,
		"subject":  map[string]any{"type": "orgUnit", "id": ouId},
	}
}

// ============================================================
// Input types
// ============================================================

type CreateGroupInput struct {
	TenantID    string
	OrgID       string
	Name        string
	Description string
	DeviceType  string // "camera" | "sensor" | "" = all
	Public      bool
	CallerID    string
}

type ListGroupsInput struct {
	TenantID   string
	OrgID      string
	Search     string
	DeviceType string
	Page       int
	PerPages   int
	SortField  string
	SortOrder  string
}

type DeleteGroupInput struct {
	TenantID string
	OrgID    string
	GroupID  string
	CallerID string
}

type AddDeviceToGroupInput struct {
	TenantID string
	OrgID    string
	GroupID  string
	DeviceID string
	CallerID string
}

type AssignGroupToOUInput struct {
	TenantID string
	OrgID    string
	GroupID  string
	OUID     string
	Relation string // viewer | editor | deleter
	CallerID string
}

type CreateDeviceInput struct {
	TenantID string
	OrgID    string
	CallerID string
	Device   devmod.Device
}

// ============================================================
// 1. CreateGroup
// ============================================================

func (s *DeviceGroupService) CreateGroup(ctx context.Context, input CreateGroupInput) (*devmod.DeviceGroup, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.OrgID = strings.TrimSpace(input.OrgID)
	input.Name = strings.TrimSpace(input.Name)
	input.CallerID = strings.TrimSpace(input.CallerID)

	if input.TenantID == "" || input.OrgID == "" || input.Name == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, err
	}

	group := &devmod.DeviceGroup{
		ID:          primitive.NewObjectID(),
		TenantID:    input.TenantID,
		OrgID:       input.OrgID,
		Name:        input.Name,
		Description: input.Description,
		DeviceType:  input.DeviceType,
		Public:      input.Public,
		CreatedBy:   input.CallerID,
		SyncStatus:  "pending",
		CreatedAt:   time.Now(),
	}

	if err := s.groupRepo.Insert(ctx, group); err != nil {
		return nil, err
	}

	groupId := group.ID.Hex()

	if err := s.authzClient.WriteTuples(ctx, input.TenantID, []map[string]any{
		tupleDeviceGroupParentOrg(groupId, input.OrgID),
	}); err != nil {
		_ = s.groupRepo.SetSyncStatus(ctx, groupId, "failed")
		return nil, fmt.Errorf("%w: %v", ErrPermifySyncFailed, err)
	}

	_ = s.groupRepo.SetSyncStatus(ctx, groupId, "synced")
	group.SyncStatus = "synced"
	return group, nil
}

// ============================================================
// 2. ListGroups
// ============================================================

func (s *DeviceGroupService) ListGroups(ctx context.Context, input ListGroupsInput) ([]devmod.DeviceGroup, int64, error) {
	return s.groupRepo.List(ctx, input.TenantID, input.OrgID, devicerepo.ListOptions{
		Search:     input.Search,
		DeviceType: input.DeviceType,
		Page:       input.Page,
		PerPages:   input.PerPages,
		SortField:  input.SortField,
		SortOrder:  input.SortOrder,
	})
}

// ============================================================
// 3. DeleteGroup
// ============================================================

func (s *DeviceGroupService) DeleteGroup(ctx context.Context, input DeleteGroupInput) error {
	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return err
	}
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, input.GroupID, input.TenantID, input.OrgID); err != nil {
		return err
	}
	if err := s.authzClient.DeleteSpecificTupleWithRelation(
		ctx, input.TenantID,
		"deviceGroup", input.GroupID, "parentOrg",
		"organization", input.OrgID,
	); err != nil {
		return fmt.Errorf("permify delete failed: %w", err)
	}
	return s.groupRepo.Delete(ctx, input.GroupID)
}
