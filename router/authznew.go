// router/authz.go
package router

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"

	"github.com/gofiber/fiber/v2"
)

func RegisterAuthzNewRoutes(router fiber.Router) {
	router.Route("/orgs", func(r fiber.Router) {

		// ✅ Audit ก่อนเสมอ (ตาม pattern ของคุณ)
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// ---------- Guards (405 protection) ----------
		r.All("/", middleware.AllowMethods("GET", "POST"))
		r.All("", middleware.AllowMethods("GET", "POST"))

		// ------------------------------------------------
		// 1️⃣ Protected (ต้องมี JWT) — selector + create
		// ------------------------------------------------
		protected := r.Group("", middleware.AuthBearer())

		// Org selector
		protected.Get("/", authznewapi.ListOrgs)
		protected.Get("", authznewapi.ListOrgs)

		// Create org (ยังไม่ต้อง ActiveOrg)
		protected.Post("/", authznewapi.CreateOrg)
		protected.Post("", authznewapi.CreateOrg)

		// ------------------------------------------------
		// 2️⃣ Org-scoped routes (ต้องมี ActiveOrg)
		// ------------------------------------------------
		// orgScoped := protected.Group("", middleware.ActiveOrg())

		// 🔥 future routes:
		// orgScoped.Get("/members", ...)
		// orgScoped.Post("/invite", ...)
		// orgScoped.Delete("/leave", ...)
	})
}
