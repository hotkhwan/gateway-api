// controllers/subapi/subscription.go
package subapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/subscriptionsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"go.opentelemetry.io/otel"
)

// SubscriptionController exposes subscription admin endpoints
type SubscriptionController struct {
	svc *subscriptionsvc.SubscriptionService
}

func NewSubscriptionController(svc *subscriptionsvc.SubscriptionService) *SubscriptionController {
	return &SubscriptionController{svc: svc}
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
func (ctrl *SubscriptionController) BootstrapSubscription(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("subapi")
	ctx, span := tracer.Start(ctx, "Subscription.Bootstrap")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)

	sub, err := ctrl.svc.BootstrapSubscription(ctx, tenantId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to bootstrap subscription",
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "subscription bootstrapped",
		"status":  true,
		"detail": fiber.Map{
			"subscriptionId": sub.IDString,
			"planId":        sub.PlanId,
			"status":        sub.Status,
			"billingCycle":  sub.BillingCycle,
		},
	})
}

// GetMySubscription returns the tenant's current subscription details
// @Summary      Get my subscription
// @Description  Returns the tenant's current subscription plan and limits
// @Tags         Subscription
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/me [get]
func (ctrl *SubscriptionController) GetMySubscription(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("subapi")
	ctx, span := tracer.Start(ctx, "Subscription.GetMySubscription")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)

	limits, err := ctrl.svc.GetTenantLimitsCached(ctx, tenantId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to get subscription",
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "subscription fetched",
		"status":  true,
		"detail": fiber.Map{
			"planId":                    limits.PlanId,
			"maxPayloadBytes":           limits.MaxPayloadBytes,
			"perOrgPerSec":             limits.PerOrgPerSec,
			"perOrgBurst":              limits.PerOrgBurst,
			"perIpPerMin":              limits.PerIpPerMin,
			"storageQuotaBytes":        limits.StorageQuotaBytes,
			"maxOrganizationsPerTenant": limits.MaxOrganizationsPerTenant,
			"orgCacheTtlSec":          limits.OrgCacheTtlSec,
		},
	})
}

// UpdatePlan updates the tenant's subscription plan (admin only)
// @Summary      Update subscription plan
// @Description  Updates the tenant's subscription to a different plan (admin only)
// @Tags         Subscription
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body subscriptionsvc.UpdatePlanRequest true "Plan update request"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      403 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/plan [patch]
func (ctrl *SubscriptionController) UpdatePlan(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("subapi")
	ctx, span := tracer.Start(ctx, "Subscription.UpdatePlan")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)

	// Parse request
	var req struct {
		PlanId       string                      `json:"planId"`
		BillingCycle subscripmod.BillingCycle  `json:"billingCycle"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid request body",
			Status:  false,
		})
	}

	// Update plan
	if err := ctrl.svc.UpdatePlan(ctx, tenantId, req.PlanId, req.BillingCycle); err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to update plan",
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "plan updated",
		"status":  true,
		"detail": fiber.Map{
			"planId":       req.PlanId,
			"billingCycle": req.BillingCycle,
		},
	})
}

// ActivateEnterprise activates enterprise plan with license key
// @Summary      Activate enterprise
// @Description  Activates enterprise plan using a license key
// @Tags         Subscription
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body subscriptionsvc.ActivateEnterpriseRequest true "Enterprise activation request"
// @Success      200 {object} gmod.SuccessDetailResponseAny
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/subscriptions/enterprise/activate [post]
func (ctrl *SubscriptionController) ActivateEnterprise(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("subapi")
	ctx, span := tracer.Start(ctx, "Subscription.ActivateEnterprise")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)

	// Parse request
	var req struct {
		LicenseKey string                        `json:"licenseKey"`
		Limits     *subscripmod.SubscriptionLimits `json:"limits,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid request body",
			Status:  false,
		})
	}

	if req.LicenseKey == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "license key is required",
			Status:  false,
		})
	}

	// Activate enterprise
	if err := ctrl.svc.ActivateEnterprise(ctx, tenantId, req.LicenseKey, req.Limits); err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to activate enterprise",
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "enterprise activated",
		"status":  true,
		"detail": fiber.Map{
			"planId": "enterprise",
		},
	})
}
