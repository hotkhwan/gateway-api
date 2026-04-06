// router/authznew.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v3"
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

		// Owner management (promote/demote)
		protected.All("/:id/owners/:userId", middleware.AllowMethods("POST", "DELETE"))
		protected.Post("/:id/owners/:userId", middleware.ActiveOrg(), c.OrgController.PromoteToOwner)
		protected.Delete("/:id/owners/:userId", middleware.ActiveOrg(), c.OrgController.DemoteFromOwner)

		// Transfer billing ownership
		protected.All("/:id/transfer-billing-ownership", middleware.AllowMethods("POST"))
		protected.Post("/:id/transfer-billing-ownership", middleware.ActiveOrg(), c.OrgController.TransferBillingOwnership)

		orgScoped := protected.Group("/units", middleware.ActiveOrg())
		orgScoped.Post("/", c.OrgUnitController.Create)
		orgScoped.Get("/tree", c.OrgUnitController.Tree)
		orgScoped.Get("/tree/:id", c.OrgUnitController.TreeNode)
		orgScoped.Patch("/:id", c.OrgUnitController.Update)
		orgScoped.Delete("/:id", c.OrgUnitController.Delete)

		protected.All("/:id/members", middleware.AllowMethods("GET", "POST", "PATCH"))
		orgScoped.Get("/:id/members", c.OrgUnitController.ListMembers)
		orgScoped.Post("/:id/members", c.OrgUnitController.AssignMembers)
		orgScoped.Patch("/:id/members", c.OrgUnitController.RemoveMembers)

		// ---------- OrgUnit ↔ ResourceGroup (list / bulk assign / bulk remove) ----------
		orgScoped.Get("/:unitId/resources", c.OrgUnitResourcesController.ListResourceGroups)
		orgScoped.Post("/:unitId/resources", c.OrgUnitResourcesController.AssignResourceGroups)
		orgScoped.Patch("/:unitId/resources", c.OrgUnitResourcesController.RemoveResourceGroups)

		// ---------- resourcePermissions CRUD (admin) ----------
		profileResourceGroup := protected.Group("/resource", middleware.ActiveOrg())
		profileResourceGroup.All("/permissions", middleware.AllowMethods("GET", "POST"))
		profileResourceGroup.All("/permissions/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		profileResourceGroup.Post("/permissions", c.ResourcePermissionsProfileController.Create)
		profileResourceGroup.Get("/permissions", c.ResourcePermissionsProfileController.List)
		profileResourceGroup.Get("/permissions/:id", c.ResourcePermissionsProfileController.GetOne)
		profileResourceGroup.Patch("/permissions/:id", c.ResourcePermissionsProfileController.Update)
		profileResourceGroup.Delete("/permissions/:id", c.ResourcePermissionsProfileController.Delete)
		// member: list resource groups accessible to the caller
		profileResourceGroup.All("/access", middleware.AllowMethods("GET"))
		profileResourceGroup.Get("/access", c.MemberAccessController.MyResourceAccess)

		// ---------- menuPermissions CRUD (admin) + member access ----------
		profileMenuGroup := protected.Group("/menu", middleware.ActiveOrg())
		profileMenuGroup.All("/list", middleware.AllowMethods("GET"))
		profileMenuGroup.All("/permissions", middleware.AllowMethods("GET", "POST"))
		profileMenuGroup.All("/permissions/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		profileMenuGroup.Get("/list", c.MenuPermissionsProfileController.ListMenus)
		profileMenuGroup.Post("/permissions", c.MenuPermissionsProfileController.Create)
		profileMenuGroup.Get("/permissions", c.MenuPermissionsProfileController.List)
		profileMenuGroup.Get("/permissions/:id", c.MenuPermissionsProfileController.GetOne)
		profileMenuGroup.Patch("/permissions/:id", c.MenuPermissionsProfileController.Update)
		profileMenuGroup.Delete("/permissions/:id", c.MenuPermissionsProfileController.Delete)
		// member: list menus accessible to the caller
		profileMenuGroup.All("/access", middleware.AllowMethods("GET"))
		profileMenuGroup.Get("/access", c.MemberAccessController.MyMenuAccess)
	})
}
