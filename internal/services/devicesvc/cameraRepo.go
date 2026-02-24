// internal/services/devicesvc/cameraRepo.go
package devicesvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/models/devmod"
)

// CameraRepo — interface ที่ devicesvc ต้องการ
// *devicerepo.CameraRepo implement interface นี้
type CameraRepo interface {
	Insert(ctx context.Context, input devmod.CreateCameraInput) (string, error)
	BulkInsert(ctx context.Context, tenantId, orgId, callerID string, items []devmod.BulkImportItem) ([]string, error)
	FindByCamID(ctx context.Context, camId string) (*devmod.CameraMongo, error)
	FindByCamIDAndOrg(ctx context.Context, camId, orgId string) (*devmod.CameraMongo, error)
	FindByCamIDs(ctx context.Context, camIds []string, search, sortField, sortOrder string, page, perPage int) ([]devmod.CameraMongo, int64, error)
	GetOrgID(ctx context.Context, camId string) (string, error)
	GetDeviceType(ctx context.Context, camId string) (string, error)
	Update(ctx context.Context, camId string, input devmod.UpdateCameraInput) error
	HardDelete(ctx context.Context, camId string) error
	Delete(ctx context.Context, camId string) error
}
