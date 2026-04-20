// router/workspaceResources.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterWorkspaceResourceRoutes mounts workspace-scoped resource CRUD.
// All routes require JWT (AuthBearer) + workspace membership (ActiveWorkspace).
//
// Delivery Targets:
//
//	POST   /workspaces/:workspaceId/delivery-targets
//	GET    /workspaces/:workspaceId/delivery-targets
//	GET    /workspaces/:workspaceId/delivery-targets/:id
//	PATCH  /workspaces/:workspaceId/delivery-targets/:id
//	DELETE /workspaces/:workspaceId/delivery-targets/:id
//
// Delivery Bindings:
//
//	POST   /workspaces/:workspaceId/delivery-bindings
//	GET    /workspaces/:workspaceId/delivery-bindings
//	GET    /workspaces/:workspaceId/delivery-bindings/:id
//	PATCH  /workspaces/:workspaceId/delivery-bindings/:id
//	DELETE /workspaces/:workspaceId/delivery-bindings/:id
//
// Ingest Templates:
//
//	POST   /workspaces/:workspaceId/ingest-templates
//	GET    /workspaces/:workspaceId/ingest-templates
//	GET    /workspaces/:workspaceId/ingest-templates/:id
//	PATCH  /workspaces/:workspaceId/ingest-templates/:id
//	DELETE /workspaces/:workspaceId/ingest-templates/:id
//
// Message Templates:
//
//	POST   /workspaces/:workspaceId/message-templates
//	GET    /workspaces/:workspaceId/message-templates
//	GET    /workspaces/:workspaceId/message-templates/:id
//	PATCH  /workspaces/:workspaceId/message-templates/:id
//	DELETE /workspaces/:workspaceId/message-templates/:id
func RegisterWorkspaceResourceRoutes(router fiber.Router, c *app.Container) {
	auditCfg := middleware.AuditConfig{
		AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
		BasePath:  os.Getenv("BASE_PATH"),
		Effective: middleware.EffectiveGetter(),
	}

	router.Route("/workspaces/:workspaceId", func(r fiber.Router) {
		r.Use(
			middleware.AuthBearer(),
			middleware.ActiveWorkspace(),
			middleware.Audit(auditCfg),
		)

		// --------- Delivery Targets ---------
		r.All("/delivery-targets", middleware.AllowMethods("GET", "POST"))
		r.All("/delivery-targets/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		r.Post("/delivery-targets", c.WsTargetController.Create)
		r.Get("/delivery-targets", c.WsTargetController.List)
		r.Get("/delivery-targets/:id", c.WsTargetController.GetOne)
		r.Patch("/delivery-targets/:id", c.WsTargetController.Update)
		r.Delete("/delivery-targets/:id", c.WsTargetController.Delete)

		// --------- Delivery Bindings ---------
		r.All("/delivery-bindings", middleware.AllowMethods("GET", "POST"))
		r.All("/delivery-bindings/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		r.Post("/delivery-bindings", c.BindingController.Create)
		r.Get("/delivery-bindings", c.BindingController.List)
		r.Get("/delivery-bindings/:id", c.BindingController.GetOne)
		r.Patch("/delivery-bindings/:id", c.BindingController.Update)
		r.Delete("/delivery-bindings/:id", c.BindingController.Delete)

		// --------- Ingest Templates ---------
		r.All("/ingest-templates", middleware.AllowMethods("GET", "POST"))
		r.All("/ingest-templates/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		r.Post("/ingest-templates", c.IngestTemplateController.Create)
		r.Get("/ingest-templates", c.IngestTemplateController.List)
		r.Get("/ingest-templates/:id", c.IngestTemplateController.GetOne)
		r.Patch("/ingest-templates/:id", c.IngestTemplateController.Update)
		r.Delete("/ingest-templates/:id", c.IngestTemplateController.Delete)

		// --------- Message Templates ---------
		r.All("/message-templates", middleware.AllowMethods("GET", "POST"))
		r.All("/message-templates/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		r.Post("/message-templates", c.MsgTemplateController.Create)
		r.Get("/message-templates", c.MsgTemplateController.List)
		r.Get("/message-templates/:id", c.MsgTemplateController.GetOne)
		r.Patch("/message-templates/:id", c.MsgTemplateController.Update)
		r.Delete("/message-templates/:id", c.MsgTemplateController.Delete)
	})
}
