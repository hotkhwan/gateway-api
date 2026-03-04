// internal/services/devicesvc/deviceGroupMembers.go
package devicesvc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/models/devmod"
)

// ============================================================
// Types
// ============================================================

type GroupDeviceItem struct {
	DeviceID string `json:"deviceId"`
}

type GroupDeviceBulkResult struct {
	DeviceID string `json:"camId"`
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
		// guard: camId ต้องมีในระบบ
		deviceOrgID, err := camRepo.GetOrgID(ctx, d.DeviceID)
		if err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "camId not found: " + d.DeviceID,
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

		// duplicate check via Permify
		isDup, err := s.authzClient.CheckPermissionWithSchemaVersion(
			ctx, tenantId, config.CurrentSchemaVersion,
			"resource", d.DeviceID, "parentGroup",
			"resourceGroup", groupId,
		)
		if err == nil && isDup {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "already in group",
			})
			duplicates++
			continue
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
		// guard: camId ต้องมีในระบบ
		deviceOrgID, err := camRepo.GetOrgID(ctx, d.DeviceID)
		if err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "camId not found: " + d.DeviceID,
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

		// check ว่า device อยู่ใน group จริงก่อนลบ
		inGroup, err := s.authzClient.CheckPermissionWithSchemaVersion(
			ctx, tenantId, config.CurrentSchemaVersion,
			"resource", d.DeviceID, "parentGroup",
			"resourceGroup", groupId,
		)
		if err != nil {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    fmt.Sprintf("permission check error: %v", err),
			})
			continue
		}
		if !inGroup {
			results = append(results, GroupDeviceBulkResult{
				DeviceID: d.DeviceID,
				Success:  false,
				Error:    "device not in group",
			})
			continue
		}

		if err := s.authzClient.DeleteSpecificTupleWithRelation(
			ctx, tenantId,
			"resource", d.DeviceID, "parentGroup",
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

// ============================================================
// List types
// ============================================================

type ListInGroupParams struct {
	Search    string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}

type ListCamerasInGroupResult struct {
	Items        []devmod.CameraDTO
	TotalRecords int64
	TotalPages   int
	Page         int
	PerPage      int
}

type ResourceGroupWithRelation struct {
	devmod.ResourceGroup
	Relation string `json:"relation"` // viewer | editor | deleter
}

type ListResourceGroupsInOUResult struct {
	Items        []ResourceGroupWithRelation
	TotalRecords int64
	TotalPages   int
	Page         int
	PerPage      int
}

// ============================================================
// ListCamerasInGroup — ดึง cameras ใน group จาก Permify แล้ว merge MongoDB
// ============================================================

func (s *ResourceGroupService) ListCamerasInGroup(
	ctx context.Context,
	tenantId, orgId, groupId string,
	params ListInGroupParams,
	camRepo CameraRepo,
) (*ListCamerasInGroupResult, error) {

	log := logger.FromCtx(ctx, "devicesvc", "ListCamerasInGroup")

	// 1) ตรวจ group ว่าอยู่ใน org
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, groupId, tenantId, orgId); err != nil {
		return nil, err
	}

	// 2) ดึง tuples ที่มี subject = resourceGroup:{groupId}
	rels, err := s.authzClient.ListRelationshipsBySubject(ctx, tenantId, "resourceGroup", groupId)
	if err != nil {
		return nil, fmt.Errorf("permify list failed: %w", err)
	}

	// 3) filter: entity.type == "resource" && relation == "parentGroup"
	camIds := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.Entity.Type == "resource" && r.Relation == "parentGroup" {
			camIds = append(camIds, r.Entity.ID)
		}
	}

	if len(camIds) == 0 {
		page := params.Page
		if page < 1 {
			page = 1
		}
		perPage := params.PerPage
		if perPage <= 0 {
			perPage = 10
		}
		return &ListCamerasInGroupResult{
			Items: []devmod.CameraDTO{}, TotalRecords: 0, TotalPages: 1,
			Page: page, PerPage: perPage,
		}, nil
	}

	// 4) fetch from MongoDB with pagination
	docs, total, err := camRepo.FindByCamIDs(ctx, camIds, params.Search, params.SortField, params.SortOrder, params.Page, params.PerPage)
	if err != nil {
		return nil, err
	}

	dtos := make([]devmod.CameraDTO, 0, len(docs))
	for _, d := range docs {
		dtos = append(dtos, devicerepo.ToDTO(&d))
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	if perPage <= 0 {
		perPage = 10
	}
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages == 0 {
		totalPages = 1
	}

	log.Info().
		Str("groupId", groupId).
		Int("camCount", len(dtos)).
		Msg("✅ ListCamerasInGroup complete")

	return &ListCamerasInGroupResult{
		Items:        dtos,
		TotalRecords: total,
		TotalPages:   totalPages,
		Page:         page,
		PerPage:      perPage,
	}, nil
}

// ============================================================
// ListResourceGroupsInOU — ดึง resourceGroups ใน OU จาก Permify แล้ว merge MongoDB
// ============================================================

func (s *ResourceGroupService) ListResourceGroupsInOU(
	ctx context.Context,
	tenantId, orgId, ouId string,
	params ListInGroupParams,
) (*ListResourceGroupsInOUResult, error) {

	log := logger.FromCtx(ctx, "devicesvc", "ListResourceGroupsInOU")

	// 1) ดึง tuples ที่มี subject = orgUnit:{ouId}
	rels, err := s.authzClient.ListRelationshipsBySubject(ctx, tenantId, "orgUnit", ouId)
	if err != nil {
		return nil, fmt.Errorf("permify list failed: %w", err)
	}

	// 2) filter: entity.type == "resourceGroup" && relation ∈ validOURelations
	// groupId → relation map (ถ้ามีหลาย relation ใช้ค่าแรก)
	groupRelMap := make(map[string]string)
	for _, r := range rels {
		if r.Entity.Type == "resourceGroup" && validOURelations[r.Relation] {
			if _, exists := groupRelMap[r.Entity.ID]; !exists {
				groupRelMap[r.Entity.ID] = r.Relation
			}
		}
	}

	groupIds := make([]string, 0, len(groupRelMap))
	for gid := range groupRelMap {
		groupIds = append(groupIds, gid)
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	if perPage <= 0 {
		perPage = 10
	}

	if len(groupIds) == 0 {
		return &ListResourceGroupsInOUResult{
			Items: []ResourceGroupWithRelation{}, TotalRecords: 0, TotalPages: 1,
			Page: page, PerPage: perPage,
		}, nil
	}

	// 3) sort groupIds for stable ordering
	sort.Strings(groupIds)

	sortField := params.SortField
	if sortField == "" {
		sortField = "createdAt"
	}

	// 4) fetch from MongoDB with pagination
	groups, total, err := s.groupRepo.FindByGroupIDs(ctx, tenantId, orgId, groupIds, devicerepo.ListOptions{
		Search:    params.Search,
		Page:      page,
		PerPages:  perPage,
		SortField: sortField,
		SortOrder: params.SortOrder,
	})
	if err != nil {
		return nil, err
	}

	items := make([]ResourceGroupWithRelation, 0, len(groups))
	for _, g := range groups {
		rel := groupRelMap[g.GroupID]
		items = append(items, ResourceGroupWithRelation{
			ResourceGroup: g,
			Relation:      strings.ToLower(rel),
		})
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if totalPages == 0 {
		totalPages = 1
	}

	log.Info().
		Str("ouId", ouId).
		Int("groupCount", len(items)).
		Msg("✅ ListResourceGroupsInOU complete")

	return &ListResourceGroupsInOUResult{
		Items:        items,
		TotalRecords: total,
		TotalPages:   totalPages,
		Page:         page,
		PerPage:      perPage,
	}, nil
}
