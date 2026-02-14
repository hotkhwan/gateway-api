// router/authz.go
package router

import (
	"klynx/config"
	"klynx/controllers/authzapi"
	"klynx/internal/middleware"
	"klynx/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v2"
)

func RegisterAuthzRoutes(router fiber.Router) {
	router.Route("/authz", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"), // หรือ os.Getenv("BASE_PATH")
			Effective: middleware.EffectiveGetter(),
		}))

		// ---------- Guards (405 สำหรับเมธอดที่ไม่รองรับ) ----------
		r.All("/healthz", middleware.AllowMethods("GET"))
		r.All("/schema/apply", middleware.AllowMethods("POST"))
		r.All("/permission/check", middleware.AllowMethods("POST"))
		r.All("/permission/check/batch", middleware.AllowMethods("POST"))
		r.All("/relationships", middleware.AllowMethods("GET"))

		r.All("/resource/grant", middleware.AllowMethods("POST"))
		r.All("/resource/revoke", middleware.AllowMethods("POST"))

		r.All("/profiles", middleware.AllowMethods("GET", "POST"))
		r.All("/profiles/:code", middleware.AllowMethods("PATCH", "DELETE"))
		//r.All("/profiles/:code/*", middleware.AllowMethods("GET", "POST")) // drift=GET, ที่เหลือ=POST
		r.All("/profiles/:code/drift", middleware.AllowMethods("GET"))
		r.All("/profiles/:code/publish", middleware.AllowMethods("POST"))
		r.All("/profiles/:code/plan", middleware.AllowMethods("POST"))
		r.All("/profiles/:code/apply", middleware.AllowMethods("POST"))
		r.All("/profiles/:code/reconcile", middleware.AllowMethods("POST"))
		// ---------- End Guards ----------

		// ---------- Handlers จริง ----------
		// Health
		r.Get("/healthz", authzapi.GetHealth)
		r.Post("/tuples/factoryResetTenant", authzapi.FactoryResetTenantHandler)
		// Schema
		r.Post("/schema/apply", authzapi.ApplySchemaHandler)

		// Permission
		r.Post("/permission/check", authzapi.PermissionCheckHandler)
		r.Post("/permission/check/batch", authzapi.PermissionCheckBatchHandler)
		r.Post("/permission/check/subjectPermission", authzapi.PermissionSubjectCheckHandler)

		// Relationships
		r.Get("/relationships", authzapi.GetRelationshipsHandler)

		// Resource
		r.Post("/resource/grant", authzapi.GrantResourceHandler)
		r.Post("/resource/revoke", authzapi.RevokeResourceHandler)

		// Profiles
		// r.Get("/profiles", authzapi.ListProfilesHandler)
		// r.Post("/profiles", authzapi.CreateProfileHandler)
		// r.Post("/profiles/:code/publish", authzapi.PublishProfileHandler)
		// r.Post("/profiles/:code/plan", authzapi.PlanProfileHandler)
		// r.Post("/profiles/:code/apply", authzapi.ApplyProfileHandler)
		// r.Get("/profiles/:code/drift", authzapi.DriftProfileHandler)
		// r.Post("/profiles/:code/reconcile", authzapi.ReconcileProfileHandler)
		// r.Patch("/profiles/:code", authzapi.UpdateProfileHandler)
		// r.Patch("/profiles/:code", middleware.AllowMethods("PATCH"), authzapi.UpdateProfileHandler)
		// r.Delete("/profiles/:code", middleware.AllowMethods("DELETE"), authzapi.DeleteProfileHandler)
		r.Get("/profiles", authzapi.ListProfilesHandler)
		r.Post("/profiles", authzapi.CreateProfileHandler)

		r.Get("/profiles/:code/drift", authzapi.DriftProfileHandler)
		r.Post("/profiles/:code/publish", authzapi.PublishProfileHandler)
		r.Post("/profiles/:code/plan", authzapi.PlanProfileHandler)
		r.Post("/profiles/:code/apply", authzapi.ApplyProfileHandler)
		r.Post("/profiles/:code/reconcile", authzapi.ReconcileProfileHandler)

		r.Patch("/profiles/:code", authzapi.UpdateProfileHandler)
		r.Delete("/profiles/:code", authzapi.DeleteProfileHandler)

	})
}
