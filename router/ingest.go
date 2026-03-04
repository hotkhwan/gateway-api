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
		// ---------- Org ingest config (admin only) ----------
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

		// Apply active organization middleware
		r.Use(middleware.ActiveOrg())

		// ---------- Event Management domain ----------
		r.Route("/management", func(mgmt fiber.Router) {
			mgmt.Get("/", c.EventManagementController.ListPendingEvents) // GET /ingest/management - รายการกิจการที่รออนนุษธิ์
			mgmt.Get("/:eventId", c.EventManagementController.GetPendingEvent) // GET /ingest/management/:eventId - รายละเอียดกิจกรรออนนุษธิ์รายการ
			mgmt.Patch("/:eventId", c.EventManagementController.UpdatePendingEvent) // PATCH /ingest/management/:eventId - อัปเดตกิจกรรออนนุษธิ์รายการ
			mgmt.Post("/:eventId/approve", c.EventManagementController.ApproveEvent) // POST /ingest/management/:eventId/approve - อนุมัตกิจกรรออนนุษธิ์รายการ
			mgmt.Post("/:eventId/reject", c.EventManagementController.RejectEvent) // POST /ingest/management/:eventId/reject - ปฏิบกิจกรรออนนุษธิ์รายการ
			mgmt.Delete("/:eventId", c.EventManagementController.DeletePendingEvent) // DELETE /ingest/management/:eventId - ลบกิจกรรออนนุษธิ์รายการ
		})

		// ---------- Event Details domain ----------
		r.Route("/details", func(details fiber.Router) {
			details.Get("/", c.EventDetailsController.ListApprovedEvents) // GET /ingest/details - รายการกิจกรอนุมัตแล้ว
			details.Get("/:eventId", c.EventDetailsController.GetApprovedEvent) // GET /ingest/details/:eventId - รายละเอียดกิจกรอนุมัตแล้วรายการ
		})
	})
}
