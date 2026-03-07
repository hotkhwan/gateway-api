// internal/services/devicemgmtsvc/service.go
package devicemgmtsvc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/cachedevicemgmt"
	"github.com/hotkhwan/gateway-api/internal/repo/devicemgmtrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type DeviceManagementService struct {
	repo *devicemgmtrepo.DeviceManagementRepo
}

func NewDeviceManagementService(repo *devicemgmtrepo.DeviceManagementRepo) *DeviceManagementService {
	if repo == nil {
		panic("DeviceManagementService: repo required")
	}
	return &DeviceManagementService{repo: repo}
}

func (s *DeviceManagementService) Create(ctx context.Context, d *ingestmod.DeviceManagement) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Create", "devicemgmtsvc", "Create")
	defer end()

	now := time.Now().UTC()
	d.DeviceMgmtId = uuid.NewString()
	d.CreatedAt = now
	d.UpdatedAt = now
	return s.repo.Insert(ctx, d)
}

func (s *DeviceManagementService) Get(ctx context.Context, tenantId, orgId, deviceMgmtId string) (*ingestmod.DeviceManagement, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Get", "devicemgmtsvc", "Get")
	defer end()

	return s.repo.FindById(ctx, tenantId, orgId, deviceMgmtId)
}

func (s *DeviceManagementService) List(ctx context.Context, tenantId, orgId string, page, perPage int) ([]*ingestmod.DeviceManagement, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "List", "devicemgmtsvc", "List")
	defer end()

	return s.repo.List(ctx, tenantId, orgId, page, perPage)
}

func (s *DeviceManagementService) Update(ctx context.Context, tenantId, orgId, deviceMgmtId string, update bson.M) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Update", "devicemgmtsvc", "Update")
	defer end()

	update["updatedAt"] = time.Now().UTC()
	if err := s.repo.Update(ctx, tenantId, orgId, deviceMgmtId, update); err != nil {
		return err
	}
	// Invalidate cache — fetch record to get entity info
	d, err := s.repo.FindById(ctx, tenantId, orgId, deviceMgmtId)
	if err == nil && d != nil {
		cachedevicemgmt.Invalidate(ctx, tenantId, orgId, d.SourceFamily, d.EntityType, d.EntityId)
	}
	return nil
}

// Resolve looks up enrichment data for a source entity (hot path, cache-first).
func (s *DeviceManagementService) Resolve(ctx context.Context, tenantId, orgId, sourceFamily, entityType, entityId string) *ingestmod.DeviceManagement {
	if cached := cachedevicemgmt.Get(ctx, tenantId, orgId, sourceFamily, entityType, entityId); cached != nil {
		return cached
	}
	d, err := s.repo.FindByEntity(ctx, tenantId, orgId, sourceFamily, entityType, entityId)
	if err != nil {
		return nil
	}
	cachedevicemgmt.Set(ctx, d)
	return d
}
