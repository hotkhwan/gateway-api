// router/targets.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterTargetsRoutes — Delivery Targets CRUD
// Mounted at /api/v1/targets (ไม่ขึ้นกับ /orgs)
// Scope: X-Active-Org header ระบุ org + JWT required + admin check ใน service
//
// POST   /api/v1/targets
// GET    /api/v1/targets
// GET    /api/v1/targets/:id
// PATCH  /api/v1/targets/:id
// DELETE /api/v1/targets/:id
func RegisterTargetsRoutes(router fiber.Router, c *app.Container) {
	router.Route("/targets", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.ActiveWorkspace())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		r.All("/", middleware.AllowMethods("GET", "POST"))
		r.All("/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))

		r.Post("/", c.TargetController.Create)
		r.Get("/", c.TargetController.List)
		r.Get("/:id", c.TargetController.GetOne)
		r.Patch("/:id", c.TargetController.Update)
		r.Delete("/:id", c.TargetController.Delete)
	})
}
