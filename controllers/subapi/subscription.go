// controllers/subapi/subscription.go
package subapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// SubscriptionController exposes subscription admin endpoints
type SubscriptionController struct {
	svc *subscriptionsvc.SubscriptionService
}

func NewSubscriptionController(svc *subscriptionsvc.SubscriptionService) *SubscriptionController {
	return &SubscriptionController{svc: svc}
}

// ListPackages returns all public subscription packages (pricing page)
// @Summary      List subscription packages
// @Description  Returns all active subscription packages ordered by sortOrder. Used for pricing/upgrade UI.
// @Tags         Subscription
// @Security     BearerAuth
// @Produce      json
// @Param        locale      query  string  false  "Locale for i18n fields"  default(en)
// @Param        publicOnly  query  bool    false  "Return only public packages"  default(true)
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/packages [get]
func (ctrl *SubscriptionController) ListPackages(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.subapi", "SubscriptionController.ListPackages", "subapi", "ListPackages")
	defer end()

	publicOnly := fiber.Query[bool](c, "publicOnly", true)

	packages, err := ctrl.svc.ListPackages(ctx, publicOnly)
	if err != nil {
		log.Error().Err(err).Msg("failed to list packages")
		return httputil.FailInternal(c, "failed to list packages")
	}

	return httputil.Ok(c, packages, "packages fetched")
}

// GetCurrentSubscription returns the tenant's effective subscription (plan + overrides resolved)
// @Summary      Get current subscription
// @Description  Returns the effective subscription with resolved limits (package + overrides merged)
// @Tags         Subscription
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/current [get]
func (ctrl *SubscriptionController) GetCurrentSubscription(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.subapi", "SubscriptionController.GetCurrentSubscription", "subapi", "GetCurrentSubscription")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	eff, err := ctrl.svc.GetCurrentEffective(ctx, tenantId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get current subscription")
		return httputil.FailInternal(c, "failed to get current subscription")
	}

	return httputil.Ok(c, eff, "current subscription fetched")
}

// BootstrapSubscription creates default freemium subscription if tenant doesn't have one
// @Summary      Bootstrap subscription
// @Description  Creates a default freemium subscription for the tenant if not already exists
// @Tags         Subscription
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/bootstrap [post]
func (ctrl *SubscriptionController) BootstrapSubscription(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.subapi", "SubscriptionController.BootstrapSubscription", "subapi", "BootstrapSubscription")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	sub, err := ctrl.svc.BootstrapSubscription(ctx, tenantId)
	if err != nil {
		log.Error().Err(err).Msg("failed to bootstrap subscription")
		return httputil.FailInternal(c, "failed to bootstrap subscription")
	}

	return httputil.Ok(c, fiber.Map{
		"subscriptionId": sub.IDString,
		"planId":         sub.PlanId,
		"status":         sub.Status,
		"billingCycle":   sub.BillingCycle,
	}, "subscription bootstrapped")
}

// UpdatePlan updates the tenant's subscription plan (admin only)
// @Summary      Update subscription plan
// @Description  Updates the tenant's subscription to a different plan (admin only)
// @Tags         Subscription
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body subscripmod.UpdatePlanRequest true "Plan update request"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      403 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/plan [patch]
func (ctrl *SubscriptionController) UpdatePlan(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.subapi", "SubscriptionController.UpdatePlan", "subapi", "UpdatePlan")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	// Parse request
	var req struct {
		PlanId       string                   `json:"planId"`
		BillingCycle subscripmod.BillingCycle `json:"billingCycle"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}

	// Update plan
	if err := ctrl.svc.UpdatePlan(ctx, tenantId, req.PlanId, req.BillingCycle); err != nil {
		log.Error().Err(err).Msg("failed to update plan")
		return httputil.FailInternal(c, "failed to update plan")
	}

	return httputil.Ok(c, fiber.Map{
		"planId":       req.PlanId,
		"billingCycle": req.BillingCycle,
	}, "plan updated")
}

// ActivateEnterprise activates enterprise plan with license key
// @Summary      Activate enterprise
// @Description  Activates enterprise plan using a license key
// @Tags         Subscription
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body subscripmod.ActivateEnterpriseRequest true "Enterprise activation request"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/enterprise/activate [post]
func (ctrl *SubscriptionController) ActivateEnterprise(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.subapi", "SubscriptionController.ActivateEnterprise", "subapi", "ActivateEnterprise")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	// Parse request
	var req struct {
		LicenseKey string                          `json:"licenseKey"`
		Limits     *subscripmod.SubscriptionLimits `json:"limits,omitempty"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}

	if req.LicenseKey == "" {
		return httputil.FailBadRequest(c, "license key is required")
	}

	// Activate enterprise
	if err := ctrl.svc.ActivateEnterprise(ctx, tenantId, req.LicenseKey, req.Limits); err != nil {
		log.Error().Err(err).Msg("failed to activate enterprise")
		return httputil.FailInternal(c, "failed to activate enterprise")
	}

	return httputil.Ok(c, fiber.Map{
		"planId": "enterprise",
	}, "enterprise activated")
}
