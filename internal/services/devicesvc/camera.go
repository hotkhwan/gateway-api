// internal/services/devicesvc/camera.go
package devicesvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/models/devmod"
)

type CameraService struct {
	repo        *devicerepo.CameraRepo
	authzClient authzgw.Client
}

func NewCameraService(repo *devicerepo.CameraRepo, authzClient authzgw.Client) *CameraService {
	return &CameraService{repo: repo, authzClient: authzClient}
}

func tupleCameraParentOrg(camId, orgId string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "resource", "id": camId},
		"relation": "parentOrg",
		"subject":  map[string]any{"type": "organization", "id": orgId},
	}
}

func tupleCameraCreator(camId, userId string) map[string]any {
	return map[string]any{
		"entity":   map[string]any{"type": "resource", "id": camId},
		"relation": "creator",
		"subject":  map[string]any{"type": "user", "id": userId},
	}
}

func (s *CameraService) guardManageOrg(ctx context.Context, tenantId, orgId, callerUserId string) error {
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", orgId, "manage", "user", callerUserId,
	)
	if err != nil {
		return fmt.Errorf("permission check error: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *CameraService) Create(ctx context.Context, input devmod.CreateCameraInput) (*devmod.CameraDTO, error) {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.Create")
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.OrgID = strings.TrimSpace(input.OrgID)
	input.Name = strings.TrimSpace(input.Name)
	input.CallerID = strings.TrimSpace(input.CallerID)
	if input.TenantID == "" || input.OrgID == "" || input.Name == "" || input.CallerID == "" {
		return nil, ErrInvalidArgs
	}
	if input.MapVisibilityOverride == "" {
		input.MapVisibilityOverride = "inherit"
	}
	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return nil, err
	}
	camId, err := s.repo.Insert(ctx, input)
	if err != nil {
		log.Error().Err(err).Msg("❌ Insert camera failed")
		return nil, err
	}
	tuples := []map[string]any{tupleCameraParentOrg(camId, input.OrgID), tupleCameraCreator(camId, input.CallerID)}
	if err := s.authzClient.WriteTuples(ctx, input.TenantID, tuples); err != nil {
		_ = s.repo.HardDelete(ctx, camId)
		log.Error().Err(err).Str("camId", camId).Msg("❌ Permify write failed — rolled back")
		return nil, fmt.Errorf("%w: %v", ErrPermifySyncFailed, err)
	}
	log.Info().Str("camId", camId).Msg("✅ Camera created")
	cam, err := s.repo.FindByCamID(ctx, camId)
	if err != nil {
		return nil, err
	}
	dto := devicerepo.ToDTO(cam)
	return &dto, nil
}

type BulkCreateResult struct {
	Inserted          int
	Failed            int
	Results           []devmod.BulkImportResult
	PermifySyncFailed []string
}

func (s *CameraService) BulkCreate(ctx context.Context, tenantId, orgId, callerID string, items []devmod.BulkImportItem) (*BulkCreateResult, error) {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.BulkCreate")
	if err := s.guardManageOrg(ctx, tenantId, orgId, callerID); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &BulkCreateResult{}, nil
	}
	ids, err := s.repo.BulkInsert(ctx, tenantId, orgId, callerID, items)
	if err != nil {
		return nil, err
	}
	tuples := make([]map[string]any, 0, len(ids)*2)
	for _, id := range ids {
		tuples = append(tuples, tupleCameraParentOrg(id, orgId), tupleCameraCreator(id, callerID))
	}
	result := &BulkCreateResult{Results: make([]devmod.BulkImportResult, 0, len(ids))}
	if permifyErr := s.authzClient.WriteTuples(ctx, tenantId, tuples); permifyErr != nil {
		log.Warn().Err(permifyErr).Msg("⚠️ Permify batch write failed")
		result.PermifySyncFailed = append(result.PermifySyncFailed, ids...)
	}
	for i, id := range ids {
		name := ""
		if i < len(items) {
			name = items[i].Name
		}
		result.Results = append(result.Results, devmod.BulkImportResult{Row: i + 2, Name: name, ID: id, Success: true})
		result.Inserted++
	}
	log.Info().Int("inserted", result.Inserted).Msg("✅ BulkCreate complete")
	return result, nil
}

func (s *CameraService) GetByID(ctx context.Context, tenantId, orgId, camId string) (*devmod.CameraDTO, error) {
	cam, err := s.repo.FindByCamIDAndOrg(ctx, camId, orgId)
	if err != nil {
		return nil, err
	}
	if cam.TenantID != tenantId {
		return nil, ErrForbidden
	}
	dto := devicerepo.ToDTO(cam)
	return &dto, nil
}

type ListCameraInput struct {
	TenantID  string
	OrgID     string
	Search    string
	GroupID   string
	Page      int
	PerPages  int
	SortField string
	SortOrder string
}

type ListCameraResult struct {
	Items   []devmod.CameraDTO
	Total   int64
	Online  int64
	Offline int64
}

func (s *CameraService) List(ctx context.Context, input ListCameraInput) (*ListCameraResult, error) {
	cameras, total, err := s.repo.List(ctx, input.TenantID, input.OrgID, devicerepo.CameraListOptions{
		Search: input.Search, GroupID: input.GroupID,
		Page: input.Page, PerPage: input.PerPages,
		SortField: input.SortField, SortOrder: input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	items := make([]devmod.CameraDTO, 0, len(cameras))
	var online, offline int64
	for _, c := range cameras {
		items = append(items, devicerepo.ToDTO(&c))
		if c.Status {
			online++
		} else {
			offline++
		}
	}
	return &ListCameraResult{Items: items, Total: total, Online: online, Offline: offline}, nil
}

func (s *CameraService) Update(ctx context.Context, tenantId, orgId, callerID, camId string, input devmod.UpdateCameraInput) error {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.Update")
	if err := s.guardManageOrg(ctx, tenantId, orgId, callerID); err != nil {
		return err
	}
	if _, err := s.repo.FindByCamIDAndOrg(ctx, camId, orgId); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, camId, input); err != nil {
		log.Error().Err(err).Str("camId", camId).Msg("❌ Update failed")
		return err
	}
	log.Info().Str("camId", camId).Msg("✅ Camera updated")
	return nil
}

func (s *CameraService) Delete(ctx context.Context, tenantId, orgId, callerID, camId string) error {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.Delete")
	if err := s.guardManageOrg(ctx, tenantId, orgId, callerID); err != nil {
		return err
	}
	if _, err := s.repo.FindByCamIDAndOrg(ctx, camId, orgId); err != nil {
		return err
	}
	// 1. Permify ก่อน
	if err := s.authzClient.DeleteEntityRelationships(ctx, tenantId, "resource", camId); err != nil {
		log.Warn().Err(err).Str("camId", camId).Msg("⚠️ Permify delete failed — proceeding")
	}
	// 2. Hard delete
	if err := s.repo.HardDelete(ctx, camId); err != nil {
		log.Error().Err(err).Str("camId", camId).Msg("❌ HardDelete failed")
		return err
	}
	log.Info().Str("camId", camId).Msg("✅ Camera hard deleted")
	return nil
}
