// router/ingest.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterIngestRoutes — mount ที่ app root (ไม่ใช่ /api/v1)
// POST /events/:orgId  — no JWT, no audit middleware
func RegisterIngestEventsRoutes(app fiber.Router, c *app.Container) {
	app.Post("/events/:orgId", c.IngestController.Ingest)

}

func RegisterIngestRoutes(router fiber.Router, c *app.Container) {
	router.Route("/ingest", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))
		protected := r.Group("/")
		// ---------- Org ingest config (admin only) ----------
		protected.All("/", middleware.AllowMethods("GET"))
		protected.All("/rotateSecret", middleware.AllowMethods("POST"))
		protected.Get("/", middleware.ActiveOrg(), c.OrgController.GetIngestConfig)
		protected.Post("/rotateSecret", middleware.ActiveOrg(), c.OrgController.RotateIngestSecret)
	})
}
