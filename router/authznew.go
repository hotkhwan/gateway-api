// router/authz.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
)

func RegisterAuthzNewRoutes(router fiber.Router) {
	authzClient := authzgw.NewClient()
	orgRepo := authzrepo.NewOrgRepo(config.DB)
	orgUnitRepo := authzrepo.NewOrgUnitRepo()
	// ===== Organization DI =====
	orgService := authzsvc.NewOrganizationService(
		orgRepo,
		orgUnitRepo,
		authzClient,
	)
	orgController := authznewapi.NewOrganizationController(orgService)

	// ===== OrgUnit DI =====
	orgUnitService := authzsvc.NewOrgUnitService(orgUnitRepo, authzClient)
	orgUnitController := authznewapi.NewOrgUnitController(orgUnitService)

	router.Route("/orgs", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		protected := r.Group("/")

		// ---------- ROOT ORG ----------
		protected.All("/", middleware.AllowMethods("GET", "POST"))
		protected.All("/:id", middleware.AllowMethods("POST", "PATCH", "DELETE"))

		protected.Get("/", orgController.List)
		protected.Post("/", orgController.Create)
		protected.Patch("/:id", orgController.Update)
		protected.Delete("/:id", orgController.Delete)

		// Invite users
		protected.All("/users/invite", middleware.AllowMethods("POST"))
		protected.Post("/users/invite", middleware.ActiveOrg(), orgController.Invite)

		// List users members
		protected.All("/users/members", middleware.AllowMethods("GET"))
		protected.Get("/users/members", middleware.ActiveOrg(), orgController.ListMembers)

		// 🔥 สำคัญ: แยก path ชัด
		orgScoped := protected.Group("/units", middleware.ActiveOrg())
		orgScoped.Post("/", orgUnitController.Create)

		// orgScoped.All("/", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		orgScoped.Get("/tree", orgUnitController.Tree)
		orgScoped.Patch("/:id", orgUnitController.Update)
		orgScoped.Delete("/:id", orgUnitController.Delete)

	})
}
