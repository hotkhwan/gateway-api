// internal/app/container.go
package app

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/adminapi"
	"github.com/hotkhwan/gateway-api/controllers/aiconfigdraftapi"
	"github.com/hotkhwan/gateway-api/controllers/aimappingapi"
	"github.com/hotkhwan/gateway-api/controllers/authzapi"
	"github.com/hotkhwan/gateway-api/controllers/bindingapi"
	"github.com/hotkhwan/gateway-api/controllers/deviceapi"
	"github.com/hotkhwan/gateway-api/controllers/ingestapi"
	"github.com/hotkhwan/gateway-api/controllers/ingesttmplapi"
	"github.com/hotkhwan/gateway-api/controllers/msgtmplapi"
	"github.com/hotkhwan/gateway-api/controllers/subapi"
	"github.com/hotkhwan/gateway-api/controllers/targetapi"
	"github.com/hotkhwan/gateway-api/controllers/workspaceapi"
	"github.com/hotkhwan/gateway-api/controllers/wstargetapi"
	"github.com/hotkhwan/gateway-api/internal/adapters/alertdispatcher"
	"github.com/hotkhwan/gateway-api/internal/crypto/secretbox"
	"github.com/hotkhwan/gateway-api/internal/eventbridge"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/grpc/eventservice"
	"github.com/hotkhwan/gateway-api/internal/kafka/deliverycons"
	"github.com/hotkhwan/gateway-api/internal/kafka/normalizedcons"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/metrics/eps"
	"github.com/hotkhwan/gateway-api/internal/mqtt/alertmsg"
	"github.com/hotkhwan/gateway-api/internal/repo/aiconfigrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/aisuggestauditrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/bindingrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/configdraftrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicemgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestmgmtrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingesttmplrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/kctrlregistryrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/msgtmplrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/rejectedpayloadpatternrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/sourceprofilerepo"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/unknownpayloadreviewrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/workspacerepo"
	"github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"
	"github.com/hotkhwan/gateway-api/internal/services/aimappingsvc"
	"github.com/hotkhwan/gateway-api/internal/services/alertdetectorsvc"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/bindingsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/internal/services/dlqsvc"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingeststatsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/internal/services/ingesttmplsvc"
	"github.com/hotkhwan/gateway-api/internal/services/kctrlregistrysvc"
	"github.com/hotkhwan/gateway-api/internal/services/licensesvc"
	"github.com/hotkhwan/gateway-api/internal/services/mappingsuggestionsvc"
	"github.com/hotkhwan/gateway-api/internal/services/msgtmplsvc"
	"github.com/hotkhwan/gateway-api/internal/services/rejectedpayloadpatternsvc"
	"github.com/hotkhwan/gateway-api/internal/services/sourceprofilesvc"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/internal/services/templatesvc"
	"github.com/hotkhwan/gateway-api/internal/services/unknownpayloadreviewsvc"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
)

// ============================================================
// Container — Composition Root
// สร้าง dependency ทั้งหมดที่นี่ที่เดียว
// router แค่รับ controller จาก container ไปผูก route
// ============================================================

type Container struct {
	// ===== Shared clients (singleton) =====
	AuthzClient   authzgw.Client
	IngestAuthzGw authzgw.IngestAuthzGateway
	IDClient      *authgw.Client

	// ===== Entitlement domain (phibek-scoped) =====
	EntitlementService *entitlementsvc.EntitlementService

	// ===== License-EPS soft metric (observe-only; nil when disabled) =====
	EPSRecorder *eps.Recorder
	EPSServer   *eps.Server

	// ===== Workspace domain (phibek-scoped) =====
	WorkspaceService               *workspacesvc.WorkspaceService
	WorkspaceMemberService         *workspacesvc.WorkspaceMemberService
	WorkspaceController            *workspaceapi.WorkspaceController
	WorkspaceMemberController      *workspaceapi.WorkspaceMemberController
	WorkspaceEntitlementController *workspaceapi.WorkspaceEntitlementController

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

	// ===== gRPC EventService (Phase C) =====
	GrpcEventRepo eventservice.EventDetailsRepo

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

	// ===== License domain (admin issuance + platform activation) =====
	LicenseService            *licensesvc.Service
	LicenseAdminController    *adminapi.LicenseAdminController
	PlatformLicenseController *adminapi.PlatformLicenseController

	// ===== Klynx camera overlay inbound (Phase B) =====
	// Per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.
	CameraOverlayInboundController *adminapi.CameraOverlayInboundController

	// Per-camera device-identity provisioning (camID Phase 1).
	// Per klynx-api/docs/contracts/dahua-camera-event-ingest.md §5.2.
	DeviceIdentityController *adminapi.DeviceIdentityController

	// ===== Klynx kcontrol registry (Phase A — receiving side) =====
	// Per klynx-api/docs/contracts/kcontrol-gw-managed-registry.md §4 + §5.
	// Service powers both the admin REST handlers and the kctrlsubmsg MQTT
	// 3-branch routing decision.
	KctrlRegistryService    *kctrlregistrysvc.Service
	KctrlRegistryController *adminapi.KctrlRegistryController

	// ===== Delivery Targets domain (org-scoped legacy) =====
	TargetService    *targetsvc.TargetService
	TargetController *targetapi.TargetController

	// ===== Workspace-scoped Delivery Targets =====
	WsTargetController *wstargetapi.WsTargetController

	// ===== Delivery Bindings domain =====
	BindingService    *bindingsvc.BindingService
	BindingController *bindingapi.BindingController

	// ===== Ingest Templates domain =====
	IngestTemplateService    *ingesttmplsvc.IngestTemplateService
	IngestTemplateController *ingesttmplapi.IngestTemplateController

	// ===== Message Templates domain =====
	MsgTemplateService    *msgtmplsvc.MsgTemplateService
	MsgTemplateController *msgtmplapi.MsgTemplateController

	// ===== AI Mapping domain =====
	AIMappingService    *aimappingsvc.AIMappingService
	AIMappingController *aimappingapi.AIMappingController

	// ===== AI Config Draft domain (Feature B) =====
	ConfigDraftService    *aiconfigdraftsvc.ConfigDraftService
	ConfigDraftController *aiconfigdraftapi.ConfigDraftController
}

// NewContainer — เรียกครั้งเดียวใน main.go
func NewContainer() *Container {
	c := &Container{}
	c.buildShared()
	c.buildEntitlement()
	c.buildWorkspace()
	c.buildAuthz()
	c.buildDevice()
	c.buildSubscription()
	c.buildLicense()
	c.buildSourceProfile()
	c.buildDeviceManagement()
	c.buildKctrlRegistry()
	c.buildRejectedPayloadPattern()
	c.buildTemplateReview()
	c.buildMappingSuggestion()
	c.buildAIMapping()
	c.buildIngest()
	c.buildAlertDispatcher()
	c.buildEvents()
	c.buildTargets()
	c.buildWorkspaceResources()
	c.buildTemplate()
	c.buildBulk()
	c.buildEPS()
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
	c.IngestAuthzGw = authzgw.NewIngestAuthzGateway(c.AuthzClient)

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
	c.OrgController = authzapi.NewOrganizationController(c.OrgService, c.WorkspaceService)

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
	c.CameraOverlayInboundController = adminapi.NewCameraOverlayInboundController(c.DeviceMgmtService)
	c.DeviceIdentityController = adminapi.NewDeviceIdentityController(c.DeviceMgmtService)
}

// buildKctrlRegistry wires the kctrl_registry surface — receives klynx-api
// Phase B PATCH/DELETE and powers the kctrlsubmsg routing decision per
// klynx-api/docs/contracts/kcontrol-gw-managed-registry.md.
//
// Phase A.1 (3.13.1+): KCTRL_REGISTRY_STRICT_MODE=true flips the §5.2 fourth
// branch on so unknown-hwId messages drop at the MQTT boundary once the
// registry has been quiet for ≥5 minutes (i.e. backfill has settled). Read
// once at boot — restart required to change.
func (c *Container) buildKctrlRegistry() {
	repo := kctrlregistryrepo.NewKctrlRegistryRepo()
	strict := strings.EqualFold(strings.TrimSpace(os.Getenv("KCTRL_REGISTRY_STRICT_MODE")), "true")
	c.KctrlRegistryService = kctrlregistrysvc.NewServiceWithOptions(repo, kctrlregistrysvc.Options{StrictMode: strict})
	c.KctrlRegistryController = adminapi.NewKctrlRegistryController(c.KctrlRegistryService)
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
	wsRepo := workspacerepo.NewWorkspaceRepo()
	eventMgmtRepo := ingestmgmtrepo.NewEventManagementRepo()

	// fingerprint template matcher
	templateRepo := ingestrepo.NewMappingTemplateRepo()
	tmplMatcher := ingestsvc.NewTemplateMatcher(templateRepo, logger.WithMeta("ingest", "template-matcher"))

	c.IngestService = ingestsvc.NewIngestService(
		wsRepo,
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
	log := logger.Boot("container", "buildSubscription")

	subRepo := subscriprepo.NewSubscriptionRepo(config.DB)
	licenseRepo := subscriprepo.NewLicenseRepo(config.DB)
	c.SubscriptionService = subscriptionsvc.NewSubscriptionService(subRepo, licenseRepo, config.Redis)
	c.SubscriptionController = subapi.NewSubscriptionController(c.SubscriptionService)

	// Let entitlementsvc overlay local tenant subscription limits onto the
	// profile catalog on cache miss (used in saasPublic only).
	if c.EntitlementService != nil {
		c.EntitlementService.SetSubscriptionResolver(c.SubscriptionService)
	}

	// Auto-bootstrap freemium subscription for default tenant on startup
	tenantId := config.PermifyTenantID
	if tenantId != "" {
		if _, err := c.SubscriptionService.BootstrapSubscription(context.Background(), tenantId); err != nil {
			log.Warn().Err(err).Str("tenantId", tenantId).Msg("failed to bootstrap default subscription")
		} else {
			log.Info().Str("tenantId", tenantId).Msg("default subscription bootstrapped")
		}
	}
}

// ============================================================
// buildLicense — enterprise license admin + platform activation
// Controllers are registered only when their feature flags are on; the
// service itself is always built so Subscription.ActivateEnterprise stays
// consistent across deployments.
// ============================================================
func (c *Container) buildLicense() {
	licenseRepo := subscriprepo.NewLicenseRepo(config.DB)
	c.LicenseService = licensesvc.New(licenseRepo)

	if envTrue("LICENSE_ADMIN_ENABLED") {
		c.LicenseAdminController = adminapi.NewLicenseAdminController(c.LicenseService)
	}
	if envTrue("PLATFORM_LICENSE_ENABLED") {
		c.PlatformLicenseController = adminapi.NewPlatformLicenseController(
			c.LicenseService,
			c.SubscriptionService,
			c.EntitlementService,
			licenseRepo,
			workspacerepo.NewWorkspaceRepo(),
		)
	}
}

// envTrue returns true when the named env var is set to "true" (case-insensitive).
func envTrue(name string) bool {
	v := os.Getenv(name)
	return v == "true" || v == "TRUE" || v == "True"
}

// ============================================================
// buildTargets — Delivery Targets domain
// ============================================================

func (c *Container) buildTargets() {
	repo := targetrepo.NewTargetRepo()
	tmplRepo := ingestrepo.NewMappingTemplateRepo()
	c.TargetService = targetsvc.NewTargetService(repo, tmplRepo, c.AuthzClient, c.SubscriptionService)
	c.TargetController = targetapi.NewTargetController(c.TargetService)
}

// ============================================================
// buildWorkspaceResources — workspace-scoped resource domains
// ============================================================

func (c *Container) buildWorkspaceResources() {
	// Workspace-scoped delivery targets — reuse TargetService
	c.WsTargetController = wstargetapi.NewWsTargetController(c.TargetService)

	// Delivery Bindings
	bindRepo := bindingrepo.NewBindingRepo()
	c.BindingService = bindingsvc.NewBindingService(bindRepo, config.Redis)
	c.BindingController = bindingapi.NewBindingController(c.BindingService)

	// Wire realtime binding cache into IngestController
	c.IngestController.SetBindingService(c.BindingService)

	// Warm realtime binding cache at startup (non-fatal)
	c.BindingService.WarmRealtimeCacheOnStartup(context.Background())

	// Ingest Templates
	ingestTmplRepo := ingesttmplrepo.NewIngestTemplateRepo()
	c.IngestTemplateService = ingesttmplsvc.NewIngestTemplateService(ingestTmplRepo)
	c.IngestTemplateController = ingesttmplapi.NewIngestTemplateController(c.IngestTemplateService)

	// Message Templates
	msgTmplRepo := msgtmplrepo.NewMsgTemplateRepo()
	c.MsgTemplateService = msgtmplsvc.NewMsgTemplateService(msgTmplRepo)
	c.MsgTemplateController = msgtmplapi.NewMsgTemplateController(c.MsgTemplateService)
}

// ============================================================
// buildEvents — Event Management domain
// ============================================================

func (c *Container) buildEvents() {
	eventMgmtRepo := ingestmgmtrepo.NewEventManagementRepo()
	eventDetailsRepo := ingestdetailsrepo.NewEventDetailsRepo()
	c.GrpcEventRepo = eventDetailsRepo

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
	profile := os.Getenv("DEPLOYMENT_PROFILE")
	isUnlimited := profile == "appliance" || profile == "enterprise"

	var ebPub normalizedcons.EventBridgePublisher
	if profile == "appliance" {
		// Inject the EPS recorder only when enabled; a true nil interface keeps
		// the publish path metric-free (avoids the nil-pointer-in-interface trap).
		var epsRec eventbridge.EPSRecorder
		if c.EPSRecorder != nil {
			epsRec = c.EPSRecorder
		}
		ebPub = eventbridge.NewKafkaEventBridgePublisher(epsRec)
	}

	tgtRepo := targetrepo.NewTargetRepo()

	// In appliance/enterprise: entitlement and authz gates are bypassed (unlimited ingest).
	// In saasPublic: gates enforce quota and source authorization.
	var entitlementSvc normalizedcons.EntitlementChecker
	var ingestAuthzGw normalizedcons.IngestAuthzChecker
	if !isUnlimited {
		entitlementSvc = c.EntitlementService
		ingestAuthzGw = c.IngestAuthzGw
	}

	c.NormalizerDeps = normalizedcons.ConsumerDeps{
		EventDetailsRepo: ingestdetailsrepo.NewEventDetailsRepo(),
		TemplateRepo:     ingestrepo.NewMappingTemplateRepo(),
		DLQRepo:          dlqrepo.NewDLQRepo(),
		GeoCfg:           normalizedcons.DefaultGeoConfig(),
		S3BucketKey: func() string {
			if v := os.Getenv("S3_BUCKET_EVENTS"); v != "" {
				return v
			}
			return "canonical"
		}(),
		Logger:             logger.WithMeta("normalizer", "consumer"),
		EntitlementSvc:     entitlementSvc,
		IngestAuthzGw:      ingestAuthzGw,
		EventBridgePub:     ebPub,
		CanonicalNotifier:  normalizedcons.NewAlertmsgNotifier(),
		BindingQuerier:     c.BindingService,
		KlynxTargetChecker: tgtRepo,
		TargetLookup:       tgtRepo,
		KlynxOrgLookup:     workspacerepo.NewWorkspaceRepo(),
		DeviceMgmtResolver: c.DeviceMgmtService,
	}
}

// ============================================================
// buildEntitlement — phibek runtime entitlement (Redis TTL cache)
// ============================================================

// workspaceTenantAdapter adapts workspacerepo.WorkspaceRepo to
// entitlementsvc.WorkspaceTenantResolver so the entitlement service can
// resolve the owning tenant on cache miss (needed on the ingest hot path
// where the consumer only has workspaceId in hand).
type workspaceTenantAdapter struct {
	repo *workspacerepo.WorkspaceRepo
}

func (a *workspaceTenantAdapter) GetTenantIDForWorkspace(ctx context.Context, workspaceID string) (string, error) {
	ws, err := a.repo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return ws.TenantID, nil
}

// ============================================================
// buildEPS — license-EPS soft metric (observe-only Prometheus export)
// ============================================================

// epsLimitAdapter adapts entitlementsvc to eps.LimitResolver: the licensed
// maxEventsPerSecond already arrives via klynx.entitlement.snapshot.v1 and is
// cached in Redis, so the collector reuses it instead of a fresh klynx pull.
type epsLimitAdapter struct {
	svc *entitlementsvc.EntitlementService
}

func (a *epsLimitAdapter) MaxEventsPerSecond(ctx context.Context, workspaceID string) (int, error) {
	ent, err := a.svc.GetWorkspaceEntitlement(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return ent.MaxEventsPerSecond, nil
}

// epsCustomerAdapter adapts workspacerepo to eps.CustomerResolver: the
// per-customer metric label is the workspace's owning tenant.
type epsCustomerAdapter struct {
	repo *workspacerepo.WorkspaceRepo
}

func (a *epsCustomerAdapter) CustomerForWorkspace(ctx context.Context, workspaceID string) (string, error) {
	ws, err := a.repo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return ws.TenantID, nil
}

// buildEPS wires the license-EPS recorder, collector, and internal /metrics
// listener. Gated behind LICENSE_EPS_METRIC_ENABLED (default off, per the
// rollback plan): when disabled, EPSRecorder/EPSServer stay nil and the
// publish path does no metric work. Must run before buildNormalizer so the
// recorder can be injected into the EventBridge publisher.
func (c *Container) buildEPS() {
	if os.Getenv("LICENSE_EPS_METRIC_ENABLED") != "true" {
		return
	}

	rec := eps.NewRecorder()

	var limits eps.LimitResolver
	if c.EntitlementService != nil {
		limits = eps.NewCachedLimitResolver(&epsLimitAdapter{svc: c.EntitlementService}, time.Minute)
	}
	customers := eps.NewCachedCustomerResolver(&epsCustomerAdapter{repo: workspacerepo.NewWorkspaceRepo()}, 5*time.Minute)

	collector := eps.NewCollector(rec, limits, customers)

	addr := os.Getenv("METRICS_ADDR")
	if addr == "" {
		addr = ":9091"
	}
	srv, err := eps.NewServer(addr, collector)
	if err != nil {
		bootLog := logger.Boot("eps", "buildEPS")
		bootLog.Error().Err(err).Msg("license-eps metrics server init failed — metric disabled")
		return
	}

	c.EPSRecorder = rec
	c.EPSServer = srv
}

func (c *Container) buildEntitlement() {
	c.EntitlementService = entitlementsvc.New(config.Redis)
	c.EntitlementService.SetWorkspaceTenantResolver(&workspaceTenantAdapter{
		repo: workspacerepo.NewWorkspaceRepo(),
	})
}

// ============================================================
// buildWorkspace — phibek workspace provisioning domain
// ============================================================

func (c *Container) buildWorkspace() {
	repo := workspacerepo.NewWorkspaceRepo()
	// In appliance mode: Permify is co-located — use pure gRPC, no REST fallback.
	// In other profiles: use HybridClient (gRPC-first with REST fallback).
	var authzWriter workspacesvc.AuthzWriter
	if os.Getenv("DEPLOYMENT_PROFILE") == "appliance" {
		authzWriter = authzgw.NewGrpcClient()
	} else {
		authzWriter = c.AuthzClient
	}
	c.WorkspaceService = workspacesvc.New(repo, authzWriter)
	c.WorkspaceMemberService = workspacesvc.NewWorkspaceMemberService(c.AuthzClient, c.IDClient)

	c.WorkspaceController = workspaceapi.NewWorkspaceController(c.WorkspaceService, c.AuthzClient)
	c.WorkspaceMemberController = workspaceapi.NewWorkspaceMemberController(c.WorkspaceMemberService)
	c.WorkspaceEntitlementController = workspaceapi.NewWorkspaceEntitlementController(c.EntitlementService)
}

// ============================================================
// buildAlertDispatcher — wires fast alert (Path A) into IngestController
// ============================================================

func (c *Container) buildAlertDispatcher() {
	det := alertdetectorsvc.New(nil) // uses DefaultAlertKeys
	disp := alertdispatcher.New(1000, 4, func(alert alertdispatcher.FastAlertEnvelope) {
		if err := alertmsg.PublishAlert(alert); err != nil {
			// non-fatal: MQTT publish failure must not affect Path B
			_ = err
		}
	})
	c.IngestController.SetAlertDispatcher(disp, det)
}

// ============================================================
// buildDeliveryConsumer — Delivery consumer deps
// ============================================================

func (c *Container) buildDeliveryConsumer() {
	c.DeliveryDeps = deliverycons.ConsumerDeps{
		TargetRepo:       targetrepo.NewTargetRepo(),
		TemplateRepo:     ingestrepo.NewMappingTemplateRepo(),
		DLQRepo:          dlqrepo.NewDLQRepo(),
		EventDetailsRepo: ingestdetailsrepo.NewEventDetailsRepo(),
		Logger:           logger.WithMeta("delivery", "consumer"),
	}
}

// ============================================================
// buildAIMapping — AI Mapping Suggest + Config Draft domains
// ============================================================

func (c *Container) buildAIMapping() {
	// Load keyring from env — fatal if missing (startup contract)
	kr, err := secretbox.LoadKeyringFromEnv()
	if err != nil {
		panic("MASTER_KEYRING_JSON required for AI mapping service: " + err.Error())
	}

	// Repos
	aiCfgRepo := aiconfigrepo.NewAIConfigRepo()
	auditRepo := aisuggestauditrepo.NewAISuggestAuditRepo()
	draftRepo := configdraftrepo.NewConfigDraftRepo()

	// AI Mapping service (Feature A)
	c.AIMappingService = aimappingsvc.NewAIMappingService(
		aiCfgRepo,
		auditRepo,
		c.MappingSuggestionService,
		config.Redis,
		kr,
	)
	c.AIMappingController = aimappingapi.NewAIMappingController(c.AIMappingService)

	// Config Draft service (Feature B)
	c.ConfigDraftService = aiconfigdraftsvc.NewConfigDraftService(draftRepo, c.MappingSuggestionService)
	c.ConfigDraftController = aiconfigdraftapi.NewConfigDraftController(c.ConfigDraftService)
}
