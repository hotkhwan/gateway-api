// internal/app/container.go
package app

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authzapi"
	"github.com/hotkhwan/gateway-api/controllers/deviceapi"
	"github.com/hotkhwan/gateway-api/controllers/eventapi"
	"github.com/hotkhwan/gateway-api/controllers/ingestapi"
	"github.com/hotkhwan/gateway-api/controllers/subapi"
	"github.com/hotkhwan/gateway-api/controllers/targetapi"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/repo/eventdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/eventmgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/internal/services/eventsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
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
	OrgService                           *authzsvc.OrganizationService
	OrgUnitService                       *authzsvc.OrgUnitService
	OrgController                        *authzapi.OrganizationController
	OrgUnitController                    *authzapi.OrgUnitController
	OrgUnitResourcesController           *authzapi.OrgUnitResourcesController
	ResourcePermissionProfileService     *authzsvc.PermissionProfileService
	ResourcePermissionsProfileController *authzapi.ResourcePermissionsProfileController
	MenuPermissionProfileService         *authzsvc.ProfileMenuPermissionService
	MenuPermissionsProfileController     *authzapi.MenuPermissionsProfileController
	MemberAccessService                  *authzsvc.MemberAccessService
	MemberAccessController               *authzapi.MemberAccessController

	// ===== Device: ResourceGroup domain ====
	ResourceGroupService    *devicesvc.ResourceGroupService
	ResourceGroupController *deviceapi.ResourceGroupController

	// ===== Device: Camera domain =====
	// resourceType = "camera" — BSON _id internal, ใช้ _id.Hex() ใน Permify
	CameraService    *devicesvc.CameraService
	CameraController *deviceapi.CameraController

	// ===== Ingest domain (hot path, no JWT) =====
	IngestService    *ingestsvc.IngestService
	IngestController *ingestapi.IngestController

	// ===== Event Management domain =====
	ApprovalService          *eventsvc.ApprovalService
	EventManagementController *eventapi.EventManagementController
	EventDetailsController    *eventapi.EventDetailsController

	// ===== Subscription domain =====
	SubscriptionService    *subscriptionsvc.SubscriptionService
	SubscriptionController *subapi.SubscriptionController

	// ===== Delivery Targets domain =====
	TargetService    *targetsvc.TargetService
	TargetController *targetapi.TargetController
}

// NewContainer — เรียกครั้งเดียวใน main.go
func NewContainer() *Container {
	c := &Container{}
	c.buildShared()
	c.buildAuthz()
	c.buildDevice()
	c.buildSubscription()
	c.buildIngest()
	c.buildEvents()
	c.buildTargets()
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
	c.OrgController = authzapi.NewOrganizationController(c.OrgService)

	c.OrgUnitService = authzsvc.NewOrgUnitService(orgUnitRepo, c.AuthzClient, c.IDClient)
	c.OrgUnitController = authzapi.NewOrgUnitController(c.OrgUnitService)
	c.OrgUnitResourcesController = authzapi.NewOrgUnitResourcesController(c.ResourceGroupService)
}

// ============================================================
// buildDevice — ResourceGroup + Camera domain
// ============================================================

func (c *Container) buildDevice() {
	groupRepo := devicerepo.NewResourceGroupRepo()
	camRepo := devicerepo.NewCameraRepo()

	// ResourceGroup (UUID external)
	c.ResourceGroupService = devicesvc.NewResourceGroupService(groupRepo, c.AuthzClient)
	c.ResourceGroupController = deviceapi.NewResourceGroupController(c.ResourceGroupService, camRepo)

	// Camera (BSON internal, _id.Hex() → Permify)
	c.CameraService = devicesvc.NewCameraService(camRepo, c.AuthzClient)
	c.CameraController = deviceapi.NewCameraController(c.CameraService)

	c.OrgUnitResourcesController = authzapi.NewOrgUnitResourcesController(c.ResourceGroupService)

	// PermissionProfile: cross-domain service (authzsvc using devicerepo)
	c.ResourcePermissionProfileService = authzsvc.NewPermissionProfileService(
		authzrepo.NewPermissionProfileRepo(),
		authzrepo.NewOrgUnitRepo(),
		groupRepo,
		c.AuthzClient,
	)
	c.ResourcePermissionsProfileController = authzapi.NewResourcePermissionsProfileController(c.ResourcePermissionProfileService)

	// MenuPermissionProfile
	menuListRepo := authzrepo.NewMenuListRepo()
	c.MenuPermissionProfileService = authzsvc.NewMenuPermissionProfileService(
		authzrepo.NewMenuPermissionProfileRepo(),
		authzrepo.NewOrgUnitRepo(),
		menuListRepo,
		c.AuthzClient,
	)
	c.MenuPermissionsProfileController = authzapi.NewMenuPermissionsProfileController(c.MenuPermissionProfileService, menuListRepo)

	// MemberAccess — read-only view of what's caller can access via OrgUnit profiles
	permProfileRepo := authzrepo.NewPermissionProfileRepo()
	menuProfileRepo := authzrepo.NewMenuPermissionProfileRepo()
	c.MemberAccessService = authzsvc.NewMemberAccessService(permProfileRepo, menuProfileRepo, c.AuthzClient, groupRepo, camRepo)
	c.MemberAccessController = authzapi.NewMemberAccessController(c.MemberAccessService)
	authzsvc.SetDefaultMemberAccessService(c.MemberAccessService)
}

// ============================================================
// buildIngest — hot-path ingest (no JWT)
// ============================================================

func (c *Container) buildIngest() {
	orgRepo := authzrepo.NewOrgRepo(config.DB)
	eventMgmtRepo := eventmgmtrepo.NewEventManagementRepo()
	c.IngestService = ingestsvc.NewIngestService(orgRepo, eventMgmtRepo, c.SubscriptionService, config.Redis, logger.WithMeta("ingest", "container"))
	c.IngestController = ingestapi.NewIngestController(c.IngestService)
}

// ============================================================
// buildSubscription — subscription service
// ============================================================
func (c *Container) buildSubscription() {
	subRepo := subscriprepo.NewSubscriptionRepo(config.DB)
	licenseRepo := subscriprepo.NewLicenseRepo(config.DB)
	c.SubscriptionService = subscriptionsvc.NewSubscriptionService(subRepo, licenseRepo, config.Redis)
	c.SubscriptionController = subapi.NewSubscriptionController(c.SubscriptionService)
}

// ============================================================
// buildTargets — Delivery Targets domain
// ============================================================

func (c *Container) buildTargets() {
	repo := targetrepo.NewTargetRepo()
	c.TargetService = targetsvc.NewTargetService(repo, c.AuthzClient)
	c.TargetController = targetapi.NewTargetController(c.TargetService)
}

// ============================================================
// buildEvents — Event Management domain
// ============================================================

func (c *Container) buildEvents() {
	eventMgmtRepo := eventmgmtrepo.NewEventManagementRepo()
	eventDetailsRepo := eventdetailsrepo.NewEventDetailsRepo()

	c.ApprovalService = eventsvc.NewApprovalService(
		eventMgmtRepo,
		eventDetailsRepo,
		config.Redis,
		logger.WithMeta("event", "approval"),
	)
	c.EventManagementController = eventapi.NewEventManagementController(c.ApprovalService)
	c.EventDetailsController = eventapi.NewEventDetailsController(c.ApprovalService)
}
