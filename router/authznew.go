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

	orgUnitRepo := authzrepo.NewOrgUnitRepo()
	authzClient := authzgw.NewClient()

	orgUnitService := authzsvc.NewOrgUnitService(
		orgUnitRepo,
		authzClient,
	)

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

		protected.Get("/", authznewapi.ListOrgs)
		protected.Post("/", authznewapi.CreateOrg)
		protected.Patch("/:id", authznewapi.UpdateOrg)
		protected.Delete("/:id", authznewapi.DeleteOrg)

		// 🔥 สำคัญ: แยก path ชัด
		orgScoped := protected.Group("/units", middleware.ActiveOrg())
		orgScoped.Post("/", orgUnitController.Create)

		// orgScoped.All("/", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		orgScoped.Get("/tree", orgUnitController.Tree)
		orgScoped.Patch("/:id", orgUnitController.Update)
		orgScoped.Delete("/:id", orgUnitController.Delete)
	})
}
