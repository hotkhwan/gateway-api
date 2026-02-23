// internal/app/container.go
package app

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/controllers/deviceapi"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
)

// ============================================================
// Container — Composition Root
// สร้าง dependency ทั้งหมดที่นี่ที่เดียว
// router แค่รับ controller จาก container ไปผูก route
// ============================================================

type Container struct {
	// ===== Shared clients (singleton) =====
	AuthzClient authzgw.Client
	IDClient    *authgw.Client

	// ===== Authz domain =====
	OrgService        *authzsvc.OrganizationService
	OrgUnitService    *authzsvc.OrgUnitService
	OrgController     *authznewapi.OrganizationController
	OrgUnitController *authznewapi.OrgUnitController

	// ===== Device: ResourceGroup domain =====
	DeviceGroupService    *devicesvc.DeviceGroupService
	DeviceGroupController *deviceapi.DeviceGroupController

	// ===== Device: Camera domain =====
	// resourceType = "camera" — BSON _id internal, ใช้ _id.Hex() ใน Permify
	CameraService    *devicesvc.CameraService
	CameraController *deviceapi.CameraController
}

// NewContainer — เรียกครั้งเดียวใน main.go
func NewContainer() *Container {
	c := &Container{}
	c.buildShared()
	c.buildAuthz()
	c.buildDevice()
	return c
}

// ============================================================
// buildShared — clients ที่ใช้ข้ามหลาย domain
// ถ้าเปลี่ยน config แก้ที่เดียว
// ============================================================

func (c *Container) buildShared() {
	c.AuthzClient = authzgw.NewClient()

	c.IDClient = authgw.New(authgw.Config{
		BaseURL:      os.Getenv("KEYCLOAK_URL"),
		Realm:        os.Getenv("KEYCLOAK_REALM"),
		ClientID:     os.Getenv("KEYCLOAK_CLIENT_ID"),
		ClientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),
	})
}

// ============================================================
// buildAuthz — Org + OrgUnit domain
// ============================================================

func (c *Container) buildAuthz() {
	orgRepo := authzrepo.NewOrgRepo(config.DB)
	orgUnitRepo := authzrepo.NewOrgUnitRepo()

	c.OrgService = authzsvc.NewOrganizationService(orgRepo, orgUnitRepo, c.AuthzClient, c.IDClient)
	c.OrgController = authznewapi.NewOrganizationController(c.OrgService)

	c.OrgUnitService = authzsvc.NewOrgUnitService(orgUnitRepo, c.AuthzClient, c.IDClient)
	c.OrgUnitController = authznewapi.NewOrgUnitController(c.OrgUnitService)
}

// ============================================================
// buildDevice — ResourceGroup + Camera domain
// ============================================================

func (c *Container) buildDevice() {
	groupRepo := devicerepo.NewDeviceGroupRepo()
	camRepo := devicerepo.NewCameraRepo()

	// ResourceGroup (UUID external)
	c.DeviceGroupService = devicesvc.NewDeviceGroupService(groupRepo, c.AuthzClient)
	c.DeviceGroupController = deviceapi.NewDeviceGroupController(c.DeviceGroupService, camRepo)

	// Camera (BSON internal, _id.Hex() → Permify)
	c.CameraService = devicesvc.NewCameraService(camRepo, c.AuthzClient)
	c.CameraController = deviceapi.NewCameraController(c.CameraService)
}
