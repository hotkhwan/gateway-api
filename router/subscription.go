// router/subscription.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

// RegisterSubscriptionRoutes — mount subscription endpoints
func RegisterSubscriptionRoutes(router fiber.Router, c *app.Container) {
	router.Route("/subscriptions", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: nil,
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// Bootstrap subscription
		r.Post("/bootstrap", c.SubscriptionController.BootstrapSubscription)

		// Get my subscription
		r.Get("/me", c.SubscriptionController.GetMySubscription)

		// Update plan (admin only)
		r.Patch("/plan", c.SubscriptionController.UpdatePlan)

		// Activate enterprise
		r.Post("/enterprise/activate", c.SubscriptionController.ActivateEnterprise)
	})
}
