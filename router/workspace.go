// router/workspace.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterWorkspaceRoutes registers all /workspaces/* REST endpoints.
// Authentication: BearerAuth (JWT) required for all routes.
// Workspace membership: X-Active-Workspace header required for workspace-scoped operations.
func RegisterWorkspaceRoutes(router fiber.Router, c *app.Container) {
	auditCfg := middleware.AuditConfig{
		AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
		BasePath:  os.Getenv("BASE_PATH"),
		Effective: middleware.EffectiveGetter(),
	}

	router.Route("/workspaces", func(r fiber.Router) {
		r.Use(
			middleware.AuthBearer(),
			middleware.Audit(auditCfg),
		)

		// ---------- Workspace CRUD (no ActiveWorkspace required — list/create are user-scoped) ----------
		r.All("/", middleware.AllowMethods("GET", "POST"))
		r.Get("/", c.WorkspaceController.List)
		r.Post("/", c.WorkspaceController.Create)

		// ---------- Workspace-scoped routes (ActiveWorkspace required) ----------
		r.All("/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		r.Get("/:id", middleware.ActiveWorkspace(), c.WorkspaceController.GetByID)
		r.Patch("/:id", middleware.ActiveWorkspace(), c.WorkspaceController.Update)
		r.Delete("/:id", middleware.ActiveWorkspace(), c.WorkspaceController.Delete)

		// ---------- Entitlement (read-only snapshot) ----------
		r.All("/entitlement", middleware.AllowMethods("GET"))
		r.Get("/entitlement", middleware.ActiveWorkspace(), c.WorkspaceEntitlementController.GetEntitlement)

		// ---------- Member management ----------
		memberGroup := r.Group("/members", middleware.ActiveWorkspace())

		memberGroup.All("/", middleware.AllowMethods("GET"))
		memberGroup.Get("/", c.WorkspaceMemberController.List)

		memberGroup.All("/invite", middleware.AllowMethods("POST"))
		memberGroup.Post("/invite", c.WorkspaceMemberController.Invite)

		memberGroup.All("/remove", middleware.AllowMethods("PATCH"))
		memberGroup.Patch("/remove", c.WorkspaceMemberController.Remove)

		memberGroup.All("/:userId/role", middleware.AllowMethods("PATCH"))
		memberGroup.Patch("/:userId/role", c.WorkspaceMemberController.ChangeRole)
	})
}
