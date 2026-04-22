// controllers/adminapi/platformLicense.go
package adminapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/repo/workspacerepo"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
	"github.com/hotkhwan/gateway-api/internal/services/licensesvc"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// PlatformLicenseController exposes the customer-facing license surface:
// validate a key before showing an activation prompt, then activate it
// against the current tenant to unlock enterprise-tier entitlements. Mounted
// only when PLATFORM_LICENSE_ENABLED=true.
type PlatformLicenseController struct {
	licSvc  *licensesvc.Service
	subSvc  *subscriptionsvc.SubscriptionService
	entSvc  *entitlementsvc.EntitlementService
	licRepo *subscriprepo.LicenseRepo
	wsRepo  *workspacerepo.WorkspaceRepo
}

func NewPlatformLicenseController(
	licSvc *licensesvc.Service,
	subSvc *subscriptionsvc.SubscriptionService,
	entSvc *entitlementsvc.EntitlementService,
	licRepo *subscriprepo.LicenseRepo,
	wsRepo *workspacerepo.WorkspaceRepo,
) *PlatformLicenseController {
	if licSvc == nil || subSvc == nil || licRepo == nil || wsRepo == nil {
		panic("PlatformLicenseController: dependencies required")
	}
	return &PlatformLicenseController{
		licSvc:  licSvc,
		subSvc:  subSvc,
		entSvc:  entSvc,
		licRepo: licRepo,
		wsRepo:  wsRepo,
	}
}

type keyPayload struct {
	LicenseKey string                          `json:"licenseKey"`
	Limits     *subscripmod.SubscriptionLimits `json:"limits,omitempty"`
}

// Current godoc
// @Summary      Get current activated license for the tenant
// @Tags         Admin.PlatformLicense
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Router       /admin/platformLicense [get]
func (ctrl *PlatformLicenseController) Current(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.adminapi", "PlatformLicenseController.Current", "adminapi", "Current")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	if tenantId == "" {
		return httputil.FailBadRequest(c, "tenant context missing")
	}

	lic, err := ctrl.licRepo.FindByTenantId(ctx, tenantId)
	if err != nil {
		if errors.Is(err, subscriprepo.ErrLicenseNotFound) {
			return httputil.FailNotFound(c, "no license activated for this tenant")
		}
		return httputil.FailInternal(c, "failed to read license")
	}
	return httputil.Ok(c, lic)
}

// Validate godoc
// @Summary      Validate a license key without activating
// @Tags         Admin.PlatformLicense
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body keyPayload true "Key payload"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Router       /admin/platformLicense/validate [post]
func (ctrl *PlatformLicenseController) Validate(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.adminapi", "PlatformLicenseController.Validate", "adminapi", "Validate")
	defer end()

	var req keyPayload
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.LicenseKey == "" {
		return httputil.FailBadRequest(c, "licenseKey is required")
	}

	lic, err := ctrl.licSvc.Validate(ctx, req.LicenseKey)
	if err != nil {
		return mapLicenseError(c, err)
	}
	return httputil.Ok(c, fiber.Map{
		"key":       lic.Key,
		"planId":    lic.PlanId,
		"status":    lic.Status,
		"createdAt": lic.CreatedAt,
	})
}

// Activate godoc
// @Summary      Activate a license key for the current tenant
// @Description  Binds the license to the tenant, upgrades the subscription to enterprise with any license-specific overrides, and invalidates runtime entitlement caches for every workspace owned by the tenant so the new limits apply on the next read without waiting for Kafka.
// @Tags         Admin.PlatformLicense
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body keyPayload true "Key payload"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Router       /admin/platformLicense/activate [post]
func (ctrl *PlatformLicenseController) Activate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "PlatformLicenseController.Activate", "adminapi", "Activate")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	if tenantId == "" {
		return httputil.FailBadRequest(c, "tenant context missing")
	}

	var req keyPayload
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.LicenseKey == "" {
		return httputil.FailBadRequest(c, "licenseKey is required")
	}

	if err := ctrl.subSvc.ActivateEnterprise(ctx, tenantId, req.LicenseKey, req.Limits); err != nil {
		log.Error().Err(err).Str("tenantId", tenantId).Msg("platform license activation failed")
		return mapLicenseError(c, err)
	}

	// Invalidate every workspace cached for this tenant so the next
	// /workspaces/entitlement read re-synthesizes with the enterprise plan
	// overlay. The route doesn't run ActiveWorkspace middleware (admins can
	// activate across tenants), so relying on c.Locals("activeWorkspace") is
	// a no-op — use the workspace directory instead.
	invalidated := 0
	if ctrl.entSvc != nil {
		workspaces, err := ctrl.wsRepo.ListByTenantID(ctx, tenantId)
		if err != nil {
			log.Warn().Err(err).Str("tenantId", tenantId).Msg("failed to list tenant workspaces for cache invalidation")
		} else {
			ids := make([]string, 0, len(workspaces))
			for _, ws := range workspaces {
				ids = append(ids, ws.WorkspaceID)
			}
			if err := ctrl.entSvc.InvalidateWorkspaces(ctx, ids); err != nil {
				log.Warn().Err(err).Str("tenantId", tenantId).Int("count", len(ids)).Msg("failed to invalidate entitlement cache for tenant workspaces")
			} else {
				invalidated = len(ids)
			}
		}
	}

	return httputil.Ok(c, fiber.Map{
		"tenantId":          tenantId,
		"planId":            "enterprise",
		"cacheInvalidated":  invalidated,
	})
}

func mapLicenseError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, licensesvc.ErrInvalidKey),
		errors.Is(err, subscriprepo.ErrLicenseNotFound):
		return httputil.FailNotFound(c, "license key not found")
	case errors.Is(err, subscriprepo.ErrLicenseAlreadyActivated):
		return httputil.FailConflict(c, "license already activated")
	case errors.Is(err, subscriprepo.ErrLicenseRevoked):
		return httputil.FailConflict(c, "license has been revoked")
	case errors.Is(err, subscriptionsvc.ErrInvalidLicenseKey):
		return httputil.FailBadRequest(c, "invalid license key")
	default:
		return httputil.FailInternal(c, "failed to process license")
	}
}
