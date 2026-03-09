// router/ingest.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterIngestEventsRoutes — hot path, no JWT, no audit
// POST /events/:orgId/:sourceFamily
func RegisterIngestEventsRoutes(app fiber.Router, c *app.Container) {
	app.Post("/events/:orgId/:sourceFamily", c.IngestController.Ingest)
}

func RegisterIngestRoutes(router fiber.Router, c *app.Container) {
	auditCfg := middleware.AuditConfig{
		AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
		BasePath:  os.Getenv("BASE_PATH"),
		Effective: middleware.EffectiveGetter(),
	}

	router.Route("/ingest", func(r fiber.Router) {
		r.Use(
			middleware.TraceHeader(),
			logger.FiberLogger(),
			middleware.AuthBearer(),
			middleware.ActiveOrg(),
			middleware.Audit(auditCfg),
		)

		// ---------- Org ingest config ----------
		r.All("/", middleware.AllowMethods("GET"))
		r.All("/rotateSecret", middleware.AllowMethods("POST"))
		r.Get("/", c.OrgController.GetIngestConfig)
		r.Post("/rotateSecret", c.OrgController.RotateIngestSecret)

		// ---------- Dashboard ----------
		r.Get("/dashboard", c.IngestDashboardController.GetIngestDashboard)

		// ---------- Management: Bulk (register before /:eventId to avoid param capture) ----------
		r.Route("/management/bulk", func(bulk fiber.Router) {
			bulk.Post("/applyTemplate", c.BulkController.BulkApplyTemplate)
			bulk.Post("/approve", c.BulkController.BulkApprove)
			bulk.Post("/reject", c.BulkController.BulkReject)
			bulk.Post("/delete", c.BulkController.BulkDelete)
		})

		// ---------- Management: Events ----------
		r.Route("/management", func(mgmt fiber.Router) {
			mgmt.Get("/", c.EventManagementController.ListPendingEvents)
			mgmt.Get("/:eventId", c.EventManagementController.GetPendingEvent)
			mgmt.Patch("/:eventId", c.EventManagementController.UpdatePendingEvent)
			mgmt.Patch("/:eventId/fieldMappings", notImplemented) // PR4: MappingTemplate CRUD
			mgmt.Post("/:eventId/approve", c.EventManagementController.ApproveEvent)
			mgmt.Post("/:eventId/reject", c.EventManagementController.RejectEvent)
			mgmt.Delete("/:eventId", c.EventManagementController.DeletePendingEvent)
		})

		// ---------- Source Profiles ----------
		r.Route("/sourceProfiles", func(sp fiber.Router) {
			sp.Get("/", c.SourceProfileController.List)
			sp.Post("/", c.SourceProfileController.Create)
			sp.Get("/:sourceFamily", c.SourceProfileController.Get)
			sp.Patch("/:sourceFamily", c.SourceProfileController.Update)
		})

		// ---------- Mapping Suggestions (read-only) ----------
		r.Route("/mappingSuggestions", func(ms fiber.Router) {
			ms.Get("/", c.MappingSuggestionController.List)
			ms.Get("/:id", c.MappingSuggestionController.Get)
		})

		// ---------- Mapping Templates ----------
		r.Route("/mappingTemplates", func(tmpl fiber.Router) {
			tmpl.Get("/", c.TemplateController.List)
			tmpl.Post("/", c.TemplateController.Create)
			tmpl.Get("/:templateId", c.TemplateController.Get)
			tmpl.Patch("/:templateId", c.TemplateController.Update)
			tmpl.Delete("/:templateId", c.TemplateController.Delete)
		})

		// ---------- Device Management ----------
		r.Route("/deviceManagement", func(dm fiber.Router) {
			dm.Get("/", c.DeviceMgmtController.List)
			dm.Post("/", c.DeviceMgmtController.Create)
			// Bulk operations — registered before /:id to avoid param capture
			dm.Get("/template", c.DeviceMgmtController.DownloadTemplate)
			dm.Get("/export", c.DeviceMgmtController.ExportXlsx)
			dm.Post("/import", c.DeviceMgmtController.ImportXlsx)
			dm.Get("/:id", c.DeviceMgmtController.Get)
			dm.Patch("/:id", c.DeviceMgmtController.Update)
		})

		// ---------- Unknown Payload Reviews ----------
		r.Route("/unknownPayloadReviews", func(tr fiber.Router) {
			tr.Get("/", c.UnknownPayloadReviewController.List)
			tr.Get("/:id", c.UnknownPayloadReviewController.Get)
			tr.Post("/:id/reject", c.UnknownPayloadReviewController.Reject)
		})

		// ---------- Rejected Payload Patterns ----------
		r.Route("/rejectedPayloadPatterns", func(rp fiber.Router) {
			rp.Get("/", c.RejectedPayloadPatternController.List)
			rp.Delete("/:id", c.RejectedPayloadPatternController.Delete)
		})

		// ---------- DLQ ----------
		r.Route("/dlq", func(dlq fiber.Router) {
			dlq.Get("/", c.DLQController.ListDLQ)
			dlq.Get("/stats", c.DLQController.GetDLQStats)
			dlq.Get("/:id", c.DLQController.GetDLQMessage)
			dlq.Post("/:id/retry", c.DLQController.RetryDLQ)
			dlq.Post("/:id/replay", c.DLQController.ReplayDLQ)
			dlq.Post("/:id/abandon", c.DLQController.AbandonDLQ)
		})

		// ---------- Details (canonical approved events — existing) ----------
		r.Route("/details", func(details fiber.Router) {
			details.Get("/", c.EventDetailsController.ListApprovedEvents)
			details.Get("/:eventId", c.EventDetailsController.GetApprovedEvent)
		})
	})
}

// notImplemented — placeholder handler จนกว่า PR ถัดไปจะ implement controller จริง
func notImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"code":    "NOT_IMPLEMENTED",
		"message": "this endpoint is not yet implemented",
		"status":  false,
	})
}
