// internal/app/container.go
package app

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authzapi"
	"github.com/hotkhwan/gateway-api/controllers/deviceapi"
	"github.com/hotkhwan/gateway-api/controllers/ingestapi"
	"github.com/hotkhwan/gateway-api/controllers/subapi"
	"github.com/hotkhwan/gateway-api/controllers/targetapi"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/kafka/deliverycons"
	"github.com/hotkhwan/gateway-api/internal/kafka/normalizedcons"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicemgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestmgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/rejectedpayloadpatternrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/sourceprofilerepo"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/unknownpayloadreviewrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/internal/services/dlqsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingeststatsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/internal/services/mappingsuggestionsvc"
	"github.com/hotkhwan/gateway-api/internal/services/rejectedpayloadpatternsvc"
	"github.com/hotkhwan/gateway-api/internal/services/sourceprofilesvc"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/internal/services/templatesvc"
	"github.com/hotkhwan/gateway-api/internal/services/unknownpayloadreviewsvc"
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
	ApprovalService           *ingestsvc.ApprovalService
	EventManagementController *ingestapi.EventManagementController
	EventDetailsController    *ingestapi.EventDetailsController

	// ===== Ingest Dashboard domain =====
	DashboardStatsService     *ingeststatsvc.DashboardStatsService
	IngestDashboardController *ingestapi.IngestDashboardController

	// ===== Source Profile domain =====
	SourceProfileService    *sourceprofilesvc.SourceProfileService
	SourceProfileController *ingestapi.SourceProfileController

	// ===== Device Management domain =====
	DeviceMgmtService    *devicemgmtsvc.DeviceManagementService
	DeviceMgmtController *ingestapi.DeviceManagementController

	// ===== Mapping Template domain =====
	TemplateController *ingestapi.TemplateController

	// ===== Mapping Suggestion domain (read-only, in-memory) =====
	MappingSuggestionService    *mappingsuggestionsvc.MappingSuggestionService
	MappingSuggestionController *ingestapi.MappingSuggestionController

	// ===== Unknown Payload Review domain =====
	UnknownPayloadReviewService    *unknownpayloadreviewsvc.UnknownPayloadReviewService
	UnknownPayloadReviewController *ingestapi.UnknownPayloadReviewController

	// ===== Rejected Payload Pattern domain =====
	RejectedPayloadPatternService    *rejectedpayloadpatternsvc.RejectedPayloadPatternService
	RejectedPayloadPatternController *ingestapi.RejectedPayloadPatternController

	// ===== DLQ domain =====
	DLQService    *dlqsvc.DLQService
	DLQController *ingestapi.DLQController

	// ===== Normalizer consumer deps =====
	NormalizerDeps normalizedcons.ConsumerDeps

	// ===== Delivery consumer deps =====
	DeliveryDeps deliverycons.ConsumerDeps

	// ===== Bulk Operations domain =====
	BulkController *ingestapi.BulkController

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
	c.buildSourceProfile()
	c.buildDeviceManagement()
	c.buildRejectedPayloadPattern()
	c.buildTemplateReview()
	c.buildMappingSuggestion()
	c.buildIngest()
	c.buildEvents()
	c.buildTargets()
	c.buildTemplate()
	c.buildBulk()
	c.buildNormalizer()
	c.buildDLQ()
	c.buildDeliveryConsumer()
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
// buildSourceProfile — Source Profile domain
// ============================================================

func (c *Container) buildSourceProfile() {
	repo := sourceprofilerepo.NewSourceProfileRepo()
	c.SourceProfileService = sourceprofilesvc.NewSourceProfileService(repo)
	c.SourceProfileController = ingestapi.NewSourceProfileController(c.SourceProfileService)
}

// ============================================================
// buildDeviceManagement — Device Management domain
// ============================================================

func (c *Container) buildDeviceManagement() {
	repo := devicemgmtrepo.NewDeviceManagementRepo()
	c.DeviceMgmtService = devicemgmtsvc.NewDeviceManagementService(repo)
	c.DeviceMgmtController = ingestapi.NewDeviceManagementController(c.DeviceMgmtService)
}

// ============================================================
// buildRejectedPayloadPattern — Rejected Payload Pattern domain
// ============================================================

func (c *Container) buildRejectedPayloadPattern() {
	repo := rejectedpayloadpatternrepo.NewRejectedPayloadPatternRepo()
	c.RejectedPayloadPatternService = rejectedpayloadpatternsvc.NewRejectedPayloadPatternService(repo)
	c.RejectedPayloadPatternController = ingestapi.NewRejectedPayloadPatternController(c.RejectedPayloadPatternService)
}

// ============================================================
// buildUnknownPayloadReview — Unknown Payload Review domain
// ============================================================

func (c *Container) buildTemplateReview() {
	repo := unknownpayloadreviewrepo.NewUnknownPayloadReviewRepo()
	c.UnknownPayloadReviewService = unknownpayloadreviewsvc.NewUnknownPayloadReviewService(repo)
	c.UnknownPayloadReviewController = ingestapi.NewUnknownPayloadReviewController(
		c.UnknownPayloadReviewService,
		c.RejectedPayloadPatternService,
	)
}

// ============================================================
// buildIngest — hot-path ingest (no JWT)
// ============================================================

func (c *Container) buildIngest() {
	orgRepo := authzrepo.NewOrgRepo(config.DB)
	eventMgmtRepo := ingestmgmtrepo.NewEventManagementRepo()

	// fingerprint template matcher
	templateRepo := ingestrepo.NewMappingTemplateRepo()
	tmplMatcher := ingestsvc.NewTemplateMatcher(templateRepo, logger.WithMeta("ingest", "template-matcher"))

	c.IngestService = ingestsvc.NewIngestService(
		orgRepo,
		eventMgmtRepo,
		c.SubscriptionService,
		config.Redis,
		tmplMatcher,
		c.SourceProfileService,
		c.UnknownPayloadReviewService,
		c.RejectedPayloadPatternService,
		c.MappingSuggestionService,
		c.DeviceMgmtService,
		logger.WithMeta("ingest", "container"),
	)
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
	c.TargetService = targetsvc.NewTargetService(repo, c.AuthzClient, c.SubscriptionService)
	c.TargetController = targetapi.NewTargetController(c.TargetService)
}

// ============================================================
// buildEvents — Event Management domain
// ============================================================

func (c *Container) buildEvents() {
	eventMgmtRepo := ingestmgmtrepo.NewEventManagementRepo()
	eventDetailsRepo := ingestdetailsrepo.NewEventDetailsRepo()

	c.ApprovalService = ingestsvc.NewApprovalService(
		eventMgmtRepo,
		eventDetailsRepo,
		config.Redis,
		logger.WithMeta("event", "approval"),
	)
	c.EventManagementController = ingestapi.NewEventManagementController(c.ApprovalService)
	c.EventDetailsController = ingestapi.NewEventDetailsController(c.ApprovalService)

	// Dashboard Stats Service
	c.DashboardStatsService = ingeststatsvc.NewDashboardStatsService(
		eventMgmtRepo,
		eventDetailsRepo,
		logger.WithMeta("ingest", "dashboard"),
	)
	c.IngestDashboardController = ingestapi.NewIngestDashboardController(c.DashboardStatsService)
}

// ============================================================
// buildTemplate — Mapping Template domain
// ============================================================

func (c *Container) buildTemplate() {
	templateRepo := ingestrepo.NewMappingTemplateRepo()
	svc := templatesvc.NewTemplateService(templateRepo)
	c.TemplateController = ingestapi.NewTemplateController(svc)
}

// ============================================================
// buildMappingSuggestion — in-memory read-only suggestions
// ============================================================

func (c *Container) buildMappingSuggestion() {
	c.MappingSuggestionService = mappingsuggestionsvc.NewMappingSuggestionService()
	c.MappingSuggestionController = ingestapi.NewMappingSuggestionController(c.MappingSuggestionService)
}

// ============================================================
// buildBulk — Bulk Operations domain (PR6)
// ============================================================

func (c *Container) buildBulk() {
	templateRepo := ingestrepo.NewMappingTemplateRepo()
	tmplMatcher := ingestsvc.NewTemplateMatcher(templateRepo, logger.WithMeta("bulk", "template-matcher"))
	svc := ingestsvc.NewBulkService(c.ApprovalService, tmplMatcher, templateRepo, logger.WithMeta("bulk", "service"))
	c.BulkController = ingestapi.NewBulkController(svc)
}

// ============================================================
// buildDLQ — DLQ service + controller
// ============================================================

func (c *Container) buildDLQ() {
	repo := dlqrepo.NewDLQRepo()
	c.DLQService = dlqsvc.NewDLQService(repo, logger.WithMeta("dlq", "service"))
	c.DLQController = ingestapi.NewDLQController(c.DLQService)
}

// ============================================================
// buildNormalizer — Normalizer consumer deps
// ============================================================

func (c *Container) buildNormalizer() {
	c.NormalizerDeps = normalizedcons.ConsumerDeps{
		EventDetailsRepo: ingestdetailsrepo.NewEventDetailsRepo(),
		TemplateRepo:     ingestrepo.NewMappingTemplateRepo(),
		DLQRepo:          dlqrepo.NewDLQRepo(),
		GeoCfg:           normalizedcons.DefaultGeoConfig(),
		S3BucketKey:      os.Getenv("S3_EVENTS_BUCKET_KEY"),
		Logger:           logger.WithMeta("normalizer", "consumer"),
	}
}

// ============================================================
// buildDeliveryConsumer — Delivery consumer deps
// ============================================================

func (c *Container) buildDeliveryConsumer() {
	c.DeliveryDeps = deliverycons.ConsumerDeps{
		TargetRepo:   targetrepo.NewTargetRepo(),
		TemplateRepo: ingestrepo.NewMappingTemplateRepo(),
		DLQRepo:      dlqrepo.NewDLQRepo(),
		Logger:       logger.WithMeta("delivery", "consumer"),
	}
}
