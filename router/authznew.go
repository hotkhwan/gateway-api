// router/authznew.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// ✅ รับ container แทนที่จะ new เอง
func RegisterAuthzNewRoutes(router fiber.Router, c *app.Container) {
	router.Route("/orgs", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		protected := r.Group("/")

		protected.All("/", middleware.AllowMethods("GET", "POST"))
		protected.All("/:id", middleware.AllowMethods("POST", "PATCH", "DELETE"))

		protected.Get("/", c.OrgController.List)
		protected.Post("/", c.OrgController.Create)
		protected.Patch("/:id", c.OrgController.Update)
		protected.Delete("/:id", c.OrgController.Delete)

		protected.All("/users/invite", middleware.AllowMethods("POST"))
		protected.Post("/users/invite", middleware.ActiveOrg(), c.OrgController.Invite)

		protected.All("/users/remove", middleware.AllowMethods("PATCH"))
		protected.Patch("/users/remove", middleware.ActiveOrg(), c.OrgController.RemoveMembers)

		protected.All("/users/members", middleware.AllowMethods("GET"))
		protected.Get("/users/members", middleware.ActiveOrg(), c.OrgController.ListMembers)

		orgScoped := protected.Group("/units", middleware.ActiveOrg())
		orgScoped.Post("/", c.OrgUnitController.Create)
		orgScoped.Get("/tree", c.OrgUnitController.Tree)
		orgScoped.Patch("/:id", c.OrgUnitController.Update)
		orgScoped.Delete("/:id", c.OrgUnitController.Delete)

		protected.All("/:id/members", middleware.AllowMethods("GET", "POST", "PATCH"))
		orgScoped.Get("/:id/members", c.OrgUnitController.ListMembers)
		orgScoped.Post("/:id/members", c.OrgUnitController.AssignMembers)
		orgScoped.Patch("/:id/members", c.OrgUnitController.RemoveMembers)
	})
}
