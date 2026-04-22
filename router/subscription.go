// router/subscription.go
package router

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

// stripeBillingEnabled returns true only in the SaaS topology when Stripe is
// explicitly switched on. gateway-api is the subscription/billing authority
// only in DEPLOYMENT_PROFILE=saasPublic; in appliance and enterprise the
// source of truth is klynx-api, so /subscriptions/*, /billing/*, and the
// Stripe webhook must not be registered here.
func stripeBillingEnabled() bool {
	if os.Getenv("DEPLOYMENT_PROFILE") != "saasPublic" {
		return false
	}
	return strings.EqualFold(os.Getenv("STRIPE_BILLING_ENABLED"), "true")
}

// RegisterSubscriptionRoutes mounts tenant-scoped subscription endpoints.
// Registered only in saasPublic when STRIPE_BILLING_ENABLED=true; other
// profiles defer to klynx-api as the subscription authority.
func RegisterSubscriptionRoutes(router fiber.Router, c *app.Container) {
	if !stripeBillingEnabled() {
		log := logger.Boot("router", "subscription")
		log.Info().
			Str("profile", os.Getenv("DEPLOYMENT_PROFILE")).
			Msg("subscription routes skipped — not saasPublic or STRIPE_BILLING_ENABLED!=true")
		return
	}

	router.Route("/subscriptions", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: nil,
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		r.All("/packages", middleware.AllowMethods("GET"))
		r.Get("/packages", c.SubscriptionController.ListPackages)

		r.All("/current", middleware.AllowMethods("GET"))
		r.Get("/current", c.SubscriptionController.GetCurrentSubscription)

		r.All("/bootstrap", middleware.AllowMethods("POST"))
		r.Post("/bootstrap", c.SubscriptionController.BootstrapSubscription)

		r.All("/plan", middleware.AllowMethods("PATCH"))
		r.Patch("/plan", c.SubscriptionController.UpdatePlan)

		r.All("/enterprise/activate", middleware.AllowMethods("POST"))
		r.Post("/enterprise/activate", c.SubscriptionController.ActivateEnterprise)
	})
}

// RegisterBillingRoutes mounts Stripe billing endpoints. Stub returning 501
// until the full Stripe port from klynx-api-feature lands (checkout session,
// onboarding, webhook verification, unsubscribe/reactivate). Route surface is
// reserved so the FE can wire against a stable path.
func RegisterBillingRoutes(router fiber.Router, _ *app.Container) {
	if !stripeBillingEnabled() {
		return
	}

	router.Route("/billing", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())

		notImplemented := func(c fiber.Ctx) error {
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
				"code":    "NOT_IMPLEMENTED",
				"message": "Stripe billing port from klynx-api-feature is pending — use klynx /billing endpoints during the migration window",
				"status":  false,
			})
		}

		r.All("/account", middleware.AllowMethods("POST"))
		r.Post("/account", notImplemented)

		r.All("/onboardingLink", middleware.AllowMethods("POST"))
		r.Post("/onboardingLink", notImplemented)

		r.All("/checkoutSession", middleware.AllowMethods("POST"))
		r.Post("/checkoutSession", notImplemented)

		r.All("/verifyCheckout", middleware.AllowMethods("POST"))
		r.Post("/verifyCheckout", notImplemented)

		r.All("/status", middleware.AllowMethods("GET"))
		r.Get("/status", notImplemented)

		r.All("/unsubscribe", middleware.AllowMethods("POST"))
		r.Post("/unsubscribe", notImplemented)

		r.All("/reactivate", middleware.AllowMethods("POST"))
		r.Post("/reactivate", notImplemented)
	})
}

// RegisterStripeWebhookRoute mounts the Stripe webhook endpoint WITHOUT
// authentication; Stripe calls it directly and signature verification is
// handled in the controller. Stub until the port lands.
func RegisterStripeWebhookRoute(router fiber.Router, _ *app.Container) {
	if !stripeBillingEnabled() {
		return
	}

	router.All("/webhooks/stripe", middleware.AllowMethods("POST"))
	router.Post("/webhooks/stripe", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"code":    "NOT_IMPLEMENTED",
			"message": "Stripe webhook handler pending — klynx-api-feature port in progress",
			"status":  false,
		})
	})
}
